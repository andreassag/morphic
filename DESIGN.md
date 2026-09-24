---
name: Morphic Design Specification
version: 2.0.0
tokens:
  colors:
    background: "#0f1117"
    surface: "#1a1d27"
    surface_elevated: "#242835"
    surface_highlight: "#2d3243"
    border: "#2e3345"
    border_hover: "#3d445c"
    text: "#e1e4ed"
    text_dim: "#8b8fa3"
    text_muted: "#5e6378"
    accent: "#6c7bf7"
    accent_hover: "#8590ff"
    accent_dim: "rgba(108, 123, 247, 0.15)"
    danger: "#f75c6c"
    danger_hover: "#ff7a87"
    danger_dim: "rgba(247, 92, 108, 0.15)"
    success: "#4ade80"
    success_dim: "rgba(74, 222, 128, 0.15)"
    warning: "#fbbf24"
    warning_dim: "rgba(251, 191, 36, 0.15)"
  typography:
    font_sans: "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif"
    font_mono: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace"
    weights:
      normal: 400
      medium: 500
      semibold: 600
      bold: 700
      heavy: 800
  spacing:
    xs: "4px"
    sm: "8px"
    md: "14px"
    lg: "20px"
    xl: "24px"
  radii:
    sm: "6px"
    md: "10px"
    lg: "14px"
---

# Morphic v2 Design Specification

## Overview
Morphic v2 provides a high-density, performant, media management dark theme optimized for large media libraries, real-time live conversion pipelines, side-by-side visual inspections, and safe file operations.

## Visual Design Language
- **Background**: Deep obsidian (`#0f1117`) paired with slate surfaces (`#1a1d27`, `#242835`) for high contrast with media assets.
- **Accents**: Vibrant electric indigo (`#6c7bf7`) for primary interactive elements and active tabs.
- **State Semantics**: Emerald (`#4ade80`) for successful conversions & live status; Ruby (`#f75c6c`) for safe-trash & destructive actions; Amber (`#fbbf24`) for warnings/conflicts.
- **Compact UI**: Single-row progress bars with embedded inline actions to maximize viewport space for media galleries and file tables.
