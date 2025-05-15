package router

import "github.com/gin-gonic/gin"

func InitRouter(porta string) {
	router := gin.Default()

    address := ":" + porta

	router.Run(address)
}
