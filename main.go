package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var wsupgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var mongoClient *mongo.Client
var chatCollection *mongo.Collection

func main() {
	// MongoDBへの接続
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var err error
	mongoClient, err = mongo.Connect(ctx, options.Client().ApplyURI("mongodb+srv:///"))
	if err != nil {
		log.Fatal("Failed to connect to MongoDB:", err)
	}
	defer mongoClient.Disconnect(ctx)

	chatCollection = mongoClient.Database("chatdb").Collection("messages")

	router := gin.Default()
	router.LoadHTMLGlob("templates/*")

	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "chat.html", nil)
	})

	router.GET("/ws", func(c *gin.Context) {
		wshandler(c.Writer, c.Request)
	})

	router.Run(":8080")
}

func wshandler(w http.ResponseWriter, r *http.Request) {
	conn, err := wsupgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "Could not open websocket connection", http.StatusBadRequest)
		return
	}
	defer conn.Close()

	sendHistory(conn) // ユーザが接続したときに履歴を送信

	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			log.Println("Read error:", err)
			break
		}
		saveMessageToDB(string(p))
		if err = conn.WriteMessage(messageType, p); err != nil {
			log.Println("Write error:", err)
			break
		}
	}
}

func sendHistory(conn *websocket.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cursor, err := chatCollection.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{"timestamp", 1}}))
	if err != nil {
		log.Println("Failed to retrieve history:", err)
		return
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var msg bson.M
		if err := cursor.Decode(&msg); err != nil {
			log.Println("Decode error:", err)
			continue
		}
		if message, ok := msg["message"].(string); ok {
			conn.WriteMessage(websocket.TextMessage, []byte(message))
		}
	}
}

func saveMessageToDB(message string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := chatCollection.InsertOne(ctx, bson.M{"message": message, "timestamp": time.Now()})
	if err != nil {
		log.Printf("Failed to save message: %v", err)
	}
}
