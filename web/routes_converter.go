package web

import (
	"net/http"
	"os"
	"time"

	"github.com/andreassag/morphic/internal/converter"
	"github.com/andreassag/morphic/internal/shared"
	"github.com/gin-gonic/gin"
)

var conversionStore = shared.NewJobStore[conversionJob]()

type conversionJob struct {
	shared.Job
	Total       int                      `json:"total"`
	Completed   int                      `json:"completed"`
	CurrentFile string                   `json:"current_file"`
	Results     []map[string]interface{} `json:"results"`
}

func init() {
	conversionStore.StartCleanup(30*time.Minute, func(j *conversionJob) time.Time {
		return j.DoneAt
	})
}

func registerConverterRoutes(r *gin.Engine) {
	g := r.Group("/api/converter")
	{
		g.POST("/scan", handleConverterScan)
		g.GET("/formats", handleConverterFormats)
		g.POST("/convert", handleConverterConvert)
		g.GET("/progress/:id", handleConverterProgress)
		g.GET("/progress/:id/poll", handleConverterPoll)
		g.POST("/progress/:id/cancel", handleConverterCancel)
		g.POST("/delete", handleConverterDelete)
	}
}

func handleConverterScan(c *gin.Context) {
	var req struct {
		Folder            string `json:"folder"`
		IncludeSubfolders *bool  `json:"include_subfolders"`
		FilterType        string `json:"filter_type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Folder == "" || !isAbsPath(req.Folder) || !isDir(req.Folder) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid folder: " + req.Folder})
		return
	}
	includeSub := true
	if req.IncludeSubfolders != nil {
		includeSub = *req.IncludeSubfolders
	}
	filterType := req.FilterType
	if filterType != "images" && filterType != "videos" && filterType != "both" {
		filterType = "both"
	}

	result, err := converter.ScanFolder(req.Folder, includeSub, filterType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func handleConverterFormats(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"image": converter.ImageConversions,
		"video": gin.H{
			"containers": converter.VideoContainers,
		},
	})
}

func handleConverterConvert(c *gin.Context) {
	var req struct {
		Files          []string `json:"files"`
		TargetExt      string   `json:"target_ext"`
		Codec          string   `json:"codec"`
		DeleteOriginal bool     `json:"delete_original"`
		AV1CRF         *int     `json:"av1_crf"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Files) == 0 || req.TargetExt == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "files and target_ext required"})
		return
	}
	if !converter.IsValidTargetExt(req.TargetExt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported or invalid target_ext: " + req.TargetExt})
		return
	}
	for _, f := range req.Files {
		if !isAbsPath(f) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file path: " + f})
			return
		}
	}

	av1CRF := 0
	if req.AV1CRF != nil {
		av1CRF = *req.AV1CRF
	}

	job := &conversionJob{
		Job:   shared.NewJob(),
		Total: len(req.Files),
	}
	job.Status = shared.JobStatusRunning
	conversionStore.Set(job.ID, job)

	go runConversion(job, req.Files, req.TargetExt, req.Codec, req.DeleteOriginal, av1CRF)

	c.JSON(http.StatusAccepted, gin.H{"job_id": job.ID})
}

func runConversion(job *conversionJob, files []string, targetExt, codec string, deleteOriginal bool, av1CRF int) {
	for i, source := range files {
		// Check for cancellation before each file
		select {
		case <-job.Ctx().Done():
			job.Status = shared.JobStatusCancelled
			job.CurrentFile = ""
			job.DoneAt = time.Now()
			return
		default:
		}

		job.CurrentFile = source

		result := map[string]interface{}{
			"source":         source,
			"source_deleted": false,
		}

		origSize := int64(0)
		if info, err := os.Stat(source); err == nil {
			origSize = info.Size()
		}

		dest, err := converter.ConvertFile(source, targetExt, codec, "", av1CRF)
		if err != nil {
			result["destination"] = nil
			result["status"] = "error"
			result["error"] = err.Error()
		} else {
			newSize := int64(0)
			if info, err := os.Stat(dest); err == nil {
				newSize = info.Size()
			}

			result["destination"] = dest
			result["status"] = "ok"
			result["original_size"] = origSize
			result["new_size"] = newSize
			result["original_size_fmt"] = shared.FormatFileSize(origSize)
			result["new_size_fmt"] = shared.FormatFileSize(newSize)

			// Delete original only if explicitly requested and safe
			if deleteOriginal && dest != "" {
				absSrc, _ := absPath(source)
				absDest, _ := absPath(dest)
				if absSrc != absDest && newSize > 0 {
					if err := os.Remove(source); err == nil {
						result["source_deleted"] = true
					}
				}
			}
		}

		job.Results = append(job.Results, result)
		job.Completed = i + 1
	}

	job.Status = shared.JobStatusDone
	job.CurrentFile = ""
	job.DoneAt = time.Now()
}

func handleConverterProgress(c *gin.Context) {
	id := c.Param("id")
	job, ok := conversionStore.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":           job.ID,
		"status":       job.Status,
		"total":        job.Total,
		"completed":    job.Completed,
		"current_file": job.CurrentFile,
		"results":      job.Results,
		"error":        job.Error,
	})
}

func handleConverterPoll(c *gin.Context) {
	id := c.Param("id")
	job, ok := conversionStore.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	lastStr := c.Query("last")
	last := -1
	if lastStr != "" {
		for i := 0; i < len(lastStr); i++ {
			if lastStr[i] >= '0' && lastStr[i] <= '9' {
				last = last*10 + int(lastStr[i]-'0')
			}
		}
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if job.Completed != last || job.Status == shared.JobStatusDone {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}

	c.JSON(http.StatusOK, gin.H{
		"id":           job.ID,
		"status":       job.Status,
		"total":        job.Total,
		"completed":    job.Completed,
		"current_file": job.CurrentFile,
		"results":      job.Results,
		"error":        job.Error,
	})
}

func handleConverterDelete(c *gin.Context) {
	var req struct {
		Files []string `json:"files"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No files specified"})
		return
	}

	var results []map[string]interface{}
	totalFreed := int64(0)

	for _, fp := range req.Files {
		if !isAbsPath(fp) {
			results = append(results, map[string]interface{}{"path": fp, "status": "not_found"})
			continue
		}
		info, err := os.Stat(fp)
		if err != nil {
			results = append(results, map[string]interface{}{"path": fp, "status": "not_found"})
			continue
		}
		if info.IsDir() {
			results = append(results, map[string]interface{}{"path": fp, "status": "not_found"})
			continue
		}
		size := info.Size()
		if err := os.Remove(fp); err != nil {
			if os.IsPermission(err) {
				results = append(results, map[string]interface{}{"path": fp, "status": "permission_denied"})
			} else {
				results = append(results, map[string]interface{}{"path": fp, "status": "error", "error": err.Error()})
			}
		} else {
			totalFreed += size
			results = append(results, map[string]interface{}{"path": fp, "status": "deleted", "size_freed": size})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"results":               results,
		"total_freed":           totalFreed,
		"total_freed_formatted": shared.FormatFileSize(totalFreed),
	})
}

func handleConverterCancel(c *gin.Context) {
	id := c.Param("id")
	job, ok := conversionStore.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}
	job.Cancel()
	c.JSON(http.StatusOK, gin.H{"status": "cancelling"})
}

func absPath(p string) (string, error) {
	abs, err := os.Getwd()
	if err != nil {
		return p, err
	}
	if len(p) > 0 && p[0] == '/' {
		return p, nil
	}
	return abs + "/" + p, nil
}
