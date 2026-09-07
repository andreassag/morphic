package web

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/exterex/morphic/internal/events"
	"github.com/gin-gonic/gin"
)

func registerSSERoutes(r *gin.Engine) {
	r.GET("/api/events/stream", handleUnifiedEventStream)
}

func handleUnifiedEventStream(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache, no-transform")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	topicsQuery := c.Query("topics")
	var topics []string
	if topicsQuery != "" {
		for _, t := range strings.Split(topicsQuery, ",") {
			if trimmed := strings.TrimSpace(t); trimmed != "" {
				topics = append(topics, trimmed)
			}
		}
	}
	if len(topics) == 0 {
		topics = []string{"*"}
	}

	eventCh := events.DefaultBus.Subscribe(c.Request.Context(), topics...)
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	c.Stream(func(w io.Writer) bool {
		select {
		case <-c.Request.Context().Done():
			return false
		case <-heartbeat.C:
			c.SSEvent("ping", gin.H{"time": time.Now().Unix()})
			w.(http.Flusher).Flush()
			return true
		case evt, ok := <-eventCh:
			if !ok {
				return false
			}
			payloadBytes, err := json.Marshal(evt.Payload)
			if err != nil {
				return true
			}
			c.SSEvent(evt.Topic, gin.H{
				"topic":     evt.Topic,
				"type":      evt.Type,
				"timestamp": evt.Timestamp.UnixMilli(),
				"data":      json.RawMessage(payloadBytes),
				"payload":   json.RawMessage(payloadBytes),
			})
			w.(http.Flusher).Flush()
			return true
		}
	})
}
