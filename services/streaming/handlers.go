package main

import (
	"github.com/gin-gonic/gin"
	"github.com/pavitra93/go-multi-tenant-system/shared/utils"
)

// handleGetStreamingStatus handles getting streaming status
func handleGetStreamingStatus(client *ThirdPartyClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := client.GetStatus()
		utils.OKResponse(c, "Streaming status retrieved successfully", status)
	}
}

// handleReconnectThirdParty handles reconnecting to third-party system
func handleReconnectThirdParty(client *ThirdPartyClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := client.Reconnect(); err != nil {
			utils.InternalServerErrorResponse(c, "Failed to reconnect: "+err.Error())
			return
		}

		utils.OKResponse(c, "Successfully reconnected to third-party system", nil)
	}
}

// handleGetStreamingMetrics handles getting streaming metrics
func handleGetStreamingMetrics(client *ThirdPartyClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := client.GetStatus()
		metrics := status["metrics"]
		utils.OKResponse(c, "Streaming metrics retrieved successfully", metrics)
	}
}
