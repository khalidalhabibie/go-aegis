package handlers

import "github.com/gin-gonic/gin"

type errorEnvelope struct {
	Error errorResponse `json:"error"`
}

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(c *gin.Context, statusCode int, code, message string) {
	c.JSON(statusCode, errorEnvelope{
		Error: errorResponse{
			Code:    code,
			Message: message,
		},
	})
}
