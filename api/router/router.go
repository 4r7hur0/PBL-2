package router

import "github.com/gin-gonic/gin"

func InitRouter() {
	router := gin.Default()

	router.Run(":8080")
}
