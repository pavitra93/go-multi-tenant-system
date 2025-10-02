package main

import (
	"github.com/gin-gonic/gin"
	"github.com/pavitra93/go-multi-tenant-system/shared/utils"
)

// handleGetStreamingHealth shows streaming service health and connection status
// This is for observability/monitoring purposes
func handleGetStreamingHealth(client *ThirdPartyClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := client.GetStatus()

		// Determine overall health
		healthy := status["connected"].(bool)

		response := map[string]interface{}{
			"service":      "streaming",
			"healthy":      healthy,
			"kafka_status": "consuming",
			"third_party":  status,
			"capabilities": []string{
				"kafka_consumer",
				"http_streaming",
				"auto_retry",
				"exponential_backoff",
			},
		}

		if healthy {
			utils.OKResponse(c, "Streaming service is healthy and connected", response)
		} else {
			c.JSON(503, map[string]interface{}{
				"success": false,
				"message": "Streaming service is unhealthy",
				"data":    response,
			})
		}
	}
}
