#!/usr/bin/env bash
# =============================================================================
# dependabot-automerge.sh
# 
# Automatically inspects and merges Dependabot pull requests if and only if
# all CI/CodeQL checks have completed and passed.
#
# Usage:
#   ./dependabot-automerge.sh [PR_NUMBER_OR_URL] [OPTIONS]
#
# Options:
#   -d, --dry-run          Inspect checks and report actions without merging
#   -w, --watch            Wait for in-progress checks to complete
#   -t, --timeout SECONDS  Max seconds to wait in watch mode (default: 600)
#   -i, --interval SECONDS Interval between poll attempts in watch mode (default: 15)
#   -m, --method METHOD    Merge method: merge | squash | rebase (default: merge)
#   -h, --help             Show this help message
# =============================================================================

set -euo pipefail

# Default configuration
DRY_RUN=false
WATCH_MODE=false
TIMEOUT_SECS=600
POLL_INTERVAL=15
MERGE_METHOD="merge"
TARGET_PR=""

# Color helpers
if [[ -t 1 ]]; then
  GREEN='\033[0;32m'
  YELLOW='\033[1;33m'
  RED='\033[0;31m'
  BLUE='\033[0;34m'
  BOLD='\033[1m'
  NC='\033[0m'
else
  GREEN=''
  YELLOW=''
  RED=''
  BLUE=''
  BOLD=''
  NC=''
fi

log_info()  { echo -e "${BLUE}[INFO]${NC} $*"; }
log_ok()    { echo -e "${GREEN}[OK]${NC} $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }

show_help() {
  cat <<EOF
Usage: $(basename "$0") [PR_NUMBER_OR_URL] [OPTIONS]

Automatically verifies CI check runs and merges Dependabot PRs when all checks pass.

Arguments:
  PR_NUMBER_OR_URL      Optional PR number (e.g. 36) or full PR URL.
                        If omitted, all open Dependabot PRs are inspected.

Options:
  -d, --dry-run         Show what would be merged without actually merging
  -w, --watch           Wait for in-progress checks to finish before deciding
  -t, --timeout SECONDS Max wait time in watch mode (default: 600)
  -i, --interval SECONDS Seconds between poll attempts (default: 15)
  -m, --method METHOD   Merge method: merge (default), squash, or rebase
  -h, --help            Show this help message
EOF
}

# Parse command line options
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      show_help
      exit 0
      ;;
    -d|--dry-run)
      DRY_RUN=true
      shift
      ;;
    -w|--watch)
      WATCH_MODE=true
      shift
      ;;
    -t|--timeout)
      TIMEOUT_SECS="$2"
      shift 2
      ;;
    -i|--interval)
      POLL_INTERVAL="$2"
      shift 2
      ;;
    -m|--method)
      MERGE_METHOD="$2"
      shift 2
      ;;
    *)
      if [[ -z "$TARGET_PR" ]]; then
        # Extract digits if full URL or #number
        TARGET_PR=$(echo "$1" | grep -oE '[0-9]+$' || echo "$1")
        shift
      else
        log_error "Unknown argument: $1"
        show_help
        exit 1
      fi
      ;;
  esac
done

# Prerequisite checks
if ! command -v gh >/dev/null 2>&1; then
  log_error "GitHub CLI (gh) is required but not installed."
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  log_error "jq is required but not installed."
  exit 1
fi

# Authenticate check (GH_TOKEN or existing auth)
if [[ -z "${GH_TOKEN:-}" && -z "${GITHUB_TOKEN:-}" ]] && ! gh auth status >/dev/null 2>&1; then
  log_error "GitHub CLI is not authenticated. Set GH_TOKEN or run 'gh auth login'."
  exit 1
fi

# Function to inspect and evaluate checks for a given PR
evaluate_pr_checks() {
  local pr_num="$1"

  gh pr view "$pr_num" --json state,number,title,baseRefName,headRefName,isDraft,mergeable,url,author,statusCheckRollup 2>/dev/null
}

# Function to analyze check rollup json
analyze_checks() {
  local pr_json="$1"

  echo "$pr_json" | jq -r '
    def is_completed:
      if .__typename == "StatusContext" then
        (.state != "PENDING")
      else
        (.status == "COMPLETED")
      end;

    def is_success:
      if .__typename == "StatusContext" then
        (.state == "SUCCESS")
      else
        (.status == "COMPLETED" and (.conclusion == "SUCCESS" or .conclusion == "NEUTRAL" or .conclusion == "SKIPPED"))
      end;

    def check_name:
      if .__typename == "StatusContext" then
        .context
      else
        .name
      end;

    def check_state:
      if .__typename == "StatusContext" then
        .state
      else
        (.conclusion // .status)
      end;

    def checks: (.statusCheckRollup // []);
    def total: (checks | length);
    def pending: [checks[] | select(is_completed | not) | "\(check_name):\(check_state)"];
    def failed: [checks[] | select(is_completed and (is_success | not)) | "\(check_name):\(check_state)"];

    {
      total: total,
      pending_count: (pending | length),
      pending_list: pending,
      failed_count: (failed | length),
      failed_list: failed,
      ready_to_merge: (total > 0 and (pending | length) == 0 and (failed | length) == 0)
    }
  '
}

process_single_pr() {
  local pr_num="$1"
  log_info "Evaluating Pull Request #${BOLD}${pr_num}${NC}..."

  local elapsed=0
  local pr_data=""
  local analysis=""

  while true; do
    pr_data=$(evaluate_pr_checks "$pr_num" || true)

    if [[ -z "$pr_data" ]]; then
      log_error "Could not fetch details for PR #${pr_num}."
      return 1
    fi

    local state title base_ref head_ref is_draft mergeable author_login
    state=$(echo "$pr_data" | jq -r '.state')
    title=$(echo "$pr_data" | jq -r '.title')
    base_ref=$(echo "$pr_data" | jq -r '.baseRefName')
    head_ref=$(echo "$pr_data" | jq -r '.headRefName')
    is_draft=$(echo "$pr_data" | jq -r '.isDraft')
    mergeable=$(echo "$pr_data" | jq -r '.mergeable')
    author_login=$(echo "$pr_data" | jq -r '.author.login')

    log_info "Title:       \"${title}\""
    log_info "State:       ${state}"
    log_info "Base Branch: ${base_ref}"
    log_info "Head Branch: ${head_ref}"
    log_info "Author:      ${author_login}"

    if [[ "$state" == "MERGED" ]]; then
      log_ok "PR #${pr_num} is already MERGED. Nothing to do."
      return 0
    fi

    if [[ "$state" == "CLOSED" ]]; then
      log_warn "PR #${pr_num} is CLOSED. Skipping."
      return 0
    fi

    # Verify Dependabot origin
    if [[ "$author_login" != "app/dependabot" && "$author_login" != "dependabot[bot]" && ! "$head_ref" =~ ^dependabot/ ]]; then
      log_warn "PR #${pr_num} does not appear to be authored by Dependabot. Skipping for safety."
      return 0
    fi

    # Draft check
    if [[ "$is_draft" == "true" ]]; then
      log_warn "PR #${pr_num} is a draft. Skipping."
      return 0
    fi

    # Conflict check
    if [[ "$mergeable" == "CONFLICTING" ]]; then
      log_error "PR #${pr_num} has merge conflicts with '${base_ref}'. Manual rebase needed. Skipping."
      return 0
    fi

    analysis=$(analyze_checks "$pr_data")
    local total pending_count failed_count ready_to_merge
    total=$(echo "$analysis" | jq -r '.total')
    pending_count=$(echo "$analysis" | jq -r '.pending_count')
    failed_count=$(echo "$analysis" | jq -r '.failed_count')
    ready_to_merge=$(echo "$analysis" | jq -r '.ready_to_merge')

    log_info "CI Status:   Total checks: ${total} | Pending: ${pending_count} | Failed: ${failed_count}"

    # If all checks passed
    if [[ "$ready_to_merge" == "true" ]]; then
      log_ok "All ${total} CI check(s) have PASSED!"
      if [[ "$DRY_RUN" == "true" ]]; then
        log_warn "[DRY RUN] Would merge PR #${pr_num} into '${base_ref}' using --${MERGE_METHOD} and delete branch '${head_ref}'."
        return 0
      fi

      log_info "Executing merge for PR #${pr_num} into '${base_ref}'..."
      if gh pr merge "$pr_num" "--${MERGE_METHOD}" --delete-branch; then
        log_ok "Successfully merged PR #${pr_num} into '${base_ref}'!"
      else
        log_error "Failed to merge PR #${pr_num}. Check branch protection or repository merge permissions."
        return 1
      fi
      return 0
    fi

    # If any checks failed
    if [[ "$failed_count" -gt 0 ]]; then
      local failed_list
      failed_list=$(echo "$analysis" | jq -r '.failed_list | join(", ")')
      log_error "PR #${pr_num} has failed check(s): ${failed_list}. Will NOT merge."
      return 0
    fi

    # Checks still in progress or none reported yet
    if [[ "$WATCH_MODE" == "true" ]]; then
      if [[ $elapsed -ge $TIMEOUT_SECS ]]; then
        log_warn "Timeout reached (${TIMEOUT_SECS}s) while waiting for PR #${pr_num} checks to finish. Skipping."
        return 0
      fi
      log_info "Checks still pending. Waiting ${POLL_INTERVAL}s (elapsed: ${elapsed}s / ${TIMEOUT_SECS}s)..."
      sleep "$POLL_INTERVAL"
      elapsed=$((elapsed + POLL_INTERVAL))
    else
      if [[ "$total" -eq 0 ]]; then
        log_warn "PR #${pr_num} has no checks reported yet. Skipping."
      else
        log_warn "PR #${pr_num} has ${pending_count} pending check(s). Use -w/--watch to wait. Skipping."
      fi
      return 0
    fi
  done
}

# Main entry point
main() {
  if [[ -n "$TARGET_PR" ]]; then
    process_single_pr "$TARGET_PR"
  else
    log_info "Searching for open Dependabot pull requests..."
    local pr_list
    pr_list=$(gh pr list --state open --json number,title,author,headRefName --jq '
      [.[] | select(.author.login == "app/dependabot" or .author.login == "dependabot[bot]" or (.headRefName | startswith("dependabot/"))) | .number] | .[]
    ')

    if [[ -z "$pr_list" ]]; then
      log_ok "No open Dependabot pull requests found."
      exit 0
    fi

    local pr_count
    pr_count=$(echo "$pr_list" | wc -l)
    log_info "Found ${pr_count} open Dependabot PR(s)."

    for pr in $pr_list; do
      echo "------------------------------------------------------------"
      process_single_pr "$pr" || true
    done
    echo "------------------------------------------------------------"
    log_ok "Finished processing Dependabot pull requests."
  fi
}

main "$@"
