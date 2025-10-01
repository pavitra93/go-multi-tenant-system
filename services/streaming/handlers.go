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

// handleGetStreamingMetrics shows streaming performance metrics
// Demonstrates failure handling and retry statistics
func handleGetStreamingMetrics(client *ThirdPartyClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := client.GetStatus()
		metrics := status["metrics"]

		response := map[string]interface{}{
			"streaming_metrics": metrics,
			"kafka_topics": []string{
				"location-updates",
				"session-events",
			},
			"retry_policy": map[string]interface{}{
				"max_retries":        3,
				"backoff_strategy":   "exponential",
				"base_delay_seconds": 1,
			},
			"protocols": []string{
				"Kafka (message broker)",
				"HTTP (third-party streaming)",
			},
		}

		utils.OKResponse(c, "Streaming metrics retrieved", response)
	}
}
