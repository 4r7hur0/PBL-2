package router

import "github.com/gin-gonic/gin"

func InitRouter(porta string) *gin.Engine{
	router := gin.Default()

  address := ":" + porta

	router.Run(address)
	
	return router
}
