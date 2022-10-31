package controllers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"backend/database"
	"backend/models"

	"github.com/gin-gonic/gin"
	"github.com/sony/sonyflake"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	//"github.com/sony/sonyflake"

	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	//run for download library `go get github.com/sony/sonyflake`
)

type Application struct {
	productCollection *mongo.Collection
	userCollection    *mongo.Collection
	orderCollection   *mongo.Collection
	payloadCollection *mongo.Collection
}

func NewApplication(productCollection *mongo.Collection, userCollection *mongo.Collection, orderCollection *mongo.Collection, payloadCollection *mongo.Collection) *Application {
	return &Application{
		productCollection: productCollection,
		userCollection:    userCollection,
		orderCollection:   orderCollection,
		payloadCollection: payloadCollection,
	}
}

func (app *Application) AddToCart() gin.HandlerFunc {
	return func(c *gin.Context) {
		var response models.Response
		productQueryId := c.Query("productId")
		if productQueryId == "" {
			log.Println("product id is empty")
			_ = c.AbortWithError(http.StatusBadRequest, errors.New("product id is empty"))
			return
		}
		userQueryId := c.Query("userId")
		if userQueryId == "" {
			log.Println("user id is empty")
			_ = c.AbortWithError(http.StatusBadRequest, errors.New("user id is empty"))
			return
		}
		productId, err := primitive.ObjectIDFromHex(productQueryId)
		if err != nil {
			log.Println(err)
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		var ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = database.AddProductToCart(ctx, app.productCollection, app.userCollection, productId, userQueryId)
		if err != nil {
			response.Status = "Failed"
			response.Code = http.StatusInternalServerError
			response.Msg = err.Error()
			c.IndentedJSON(http.StatusInternalServerError, response)
			return
		}

		response.Status = "OK"
		response.Code = 200
		response.Msg = "Successfully added to the cart"
		c.IndentedJSON(200, response)
		return
	}
}

func (app *Application) RemoveItem() gin.HandlerFunc {
	return func(c *gin.Context) {
		var response models.Response
		productQueryId := c.Query("productId")
		if productQueryId == "" {
			log.Println("Missing product id")
			_ = c.AbortWithError(http.StatusBadRequest, errors.New("Missing product id"))
			return
		}

		userQueryId := c.Query("userId")
		if userQueryId == "" {
			log.Println("Missing user id")
			_ = c.AbortWithError(http.StatusBadRequest, errors.New("Missing user id"))
		}

		productId, err := primitive.ObjectIDFromHex(productQueryId)
		if err != nil {
			log.Println(err)
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		var ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = database.RemoveCartItem(ctx, app.productCollection, app.userCollection, productId, userQueryId)
		if err != nil {
			response.Status = "Failed"
			response.Code = http.StatusInternalServerError
			response.Msg = err.Error()
			c.IndentedJSON(http.StatusInternalServerError, response)
			return
		}

		response.Status = "OK"
		response.Code = 200
		response.Msg = "Successfully removed from cart"
		c.IndentedJSON(200, response)
		return
	}
}

func GetItemsFromCart() gin.HandlerFunc {
	return func(c *gin.Context) {
		var response models.Response
		userId := c.Query("userId")
		if userId == "" {
			c.Header("Content-Type", "application/json")

			response.Status = "Failed"
			response.Code = http.StatusNotFound
			response.Msg = "Missing userId"
			c.IndentedJSON(http.StatusNotFound, response)
			c.Abort()
			return
		}

		usertId, _ := primitive.ObjectIDFromHex(userId)

		var ctx, cancel = context.WithTimeout(context.Background(), 100*time.Second)
		defer cancel()

		var filledCart models.User
		err := UserCollection.FindOne(ctx, bson.D{primitive.E{Key: "_id", Value: usertId}}).Decode(&filledCart)
		if err != nil {
			log.Println(err)

			response.Status = "Failed"
			response.Code = 500
			response.Msg = "User id not found"
			c.IndentedJSON(500, response)
			return
		}

		filterMatch := bson.D{{Key: "$match", Value: bson.D{primitive.E{Key: "_id", Value: usertId}}}}
		unwind := bson.D{{Key: "$unwind", Value: bson.D{primitive.E{Key: "path", Value: "$user_cart"}}}}
		grouping := bson.D{{Key: "$group", Value: bson.D{primitive.E{Key: "_id", Value: "$_id"}, {Key: "total", Value: bson.D{primitive.E{Key: "$sum", Value: "$user_cart.price"}}}}}}
		pointCursor, err := UserCollection.Aggregate(ctx, mongo.Pipeline{filterMatch, unwind, grouping})
		if err != nil {
			log.Println(err)
		}
		var listing []bson.M
		if err = pointCursor.All(ctx, &listing); err != nil {
			log.Println(err)
			c.AbortWithStatus(http.StatusInternalServerError)
		}

		for _, json := range listing {
			response.Status = "OK"
			response.Code = 200
			response.Msg = json["total"]
			response.Data = filledCart.UserCart
			c.IndentedJSON(200, response)
			return
		}

		ctx.Done()
	}
}

func (app *Application) BuyFromCart() gin.HandlerFunc {
	return func(c *gin.Context) {
		var response models.Response
		userQueryId := c.Query("userId")
		if userQueryId == "" {
			log.Panicln("user id is empty")
			_ = c.AbortWithError(http.StatusBadRequest, errors.New("UserID is empty"))
		}
		var ctx, cancel = context.WithTimeout(context.Background(), 100*time.Second)
		defer cancel()
		err := database.BuyItemFromCart(ctx, app.userCollection, userQueryId, app.orderCollection)
		if err != nil {
			response.Status = "Failed"
			response.Code = http.StatusInternalServerError
			response.Msg = err.Error()
			c.IndentedJSON(http.StatusInternalServerError, response)
			return
		}

		response.Status = "OK"
		response.Code = 200
		response.Msg = "Successfully placed the order"
		c.IndentedJSON(200, response)
		return
	}
}

// func (app *Application) InstantBuy() gin.HandlerFunc {
// 	return func(c *gin.Context) {
// 		var response models.Response
// 		userQueryId := c.Query("userId")
// 		if userQueryId == "" {
// 			log.Println("UserID is empty")
// 			_ = c.AbortWithError(http.StatusBadRequest, errors.New("UserID is empty"))
// 		}
// 		productQueryId := c.Query("productId")
// 		if productQueryId == "" {
// 			log.Println("Product_ID id is empty")
// 			_ = c.AbortWithError(http.StatusBadRequest, errors.New("product_id is empty"))
// 		}
// 		productId, err := primitive.ObjectIDFromHex(productQueryId)
// 		if err != nil {
// 			log.Println(err)
// 			c.AbortWithStatus(http.StatusInternalServerError)
// 			return
// 		}

// 		var ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
// 		defer cancel()
// 		err = database.InstantBuyer(ctx, app.productCollection, app.userCollection, productId, userQueryId)
// 		if err != nil {
// 			response.Status = "Failed"
// 			response.Code = http.StatusInternalServerError
// 			response.Msg = err.Error()
// 			c.IndentedJSON(http.StatusInternalServerError, response)
// 			return
// 		}

// 		response.Status = "OK"
// 		response.Code = 200
// 		response.Msg = "Successfully placed the order"
// 		c.IndentedJSON(200, response)
// 		return
// 	}
// }

func PaymentOrders() gin.HandlerFunc {
	return func(c *gin.Context) {
		var response models.Response
		var ctx, cancel = context.WithTimeout(context.Background(), 100*time.Second)
		defer cancel()
		var payload models.Payload
		if err := c.BindJSON(&payload); err != nil {
			response.Status = "Failed"
			response.Code = http.StatusBadRequest
			response.Msg = err.Error()
			c.IndentedJSON(http.StatusBadRequest, response)
			return
		}
		flake := sonyflake.NewSonyflake(sonyflake.Settings{})
		//randome orderID and requestID
		a, err := flake.NextID()
		var requestId = strconv.FormatUint(a, 16)
		var secretKey = "PIAHFSUXSXISQXS2wrdpH0Vl1d7sdGTO"
		payload.RequestID = requestId
		payload.ExtraData = ""
		payload.PartnerCode = "MOMOIHKM20221002"
		payload.AccessKey = "rq9CyDN11E1Z56iT"
		payload.RequestType = "captureWallet"
		payload.RedirectUrl = "http://localhost:8000/user/view-payment" //return page success payment
		payload.IpnUrl = "http://localhost:8000/user/view-payment"

		var rawSignature bytes.Buffer
		rawSignature.WriteString("accessKey=")
		rawSignature.WriteString(payload.AccessKey)
		rawSignature.WriteString("&amount=")
		rawSignature.WriteString(strconv.FormatUint(payload.Amount, 10))
		rawSignature.WriteString("&extraData=")
		rawSignature.WriteString(payload.ExtraData)
		rawSignature.WriteString("&ipnUrl=")
		rawSignature.WriteString(payload.IpnUrl)
		rawSignature.WriteString("&orderId=")
		rawSignature.WriteString(payload.OrderID)
		rawSignature.WriteString("&orderInfo=")
		rawSignature.WriteString(payload.OrderInfo)
		rawSignature.WriteString("&partnerCode=")
		rawSignature.WriteString(payload.PartnerCode)
		rawSignature.WriteString("&redirectUrl=")
		rawSignature.WriteString(payload.RedirectUrl)
		rawSignature.WriteString("&requestId=")
		rawSignature.WriteString(payload.RequestID)
		rawSignature.WriteString("&requestType=")
		rawSignature.WriteString(payload.RequestType)

		hmac := hmac.New(sha256.New, []byte(secretKey))

		// Write Data to it
		hmac.Write(rawSignature.Bytes())
		fmt.Println("Raw signature: " + rawSignature.String())

		// Get result and encode as hexadecimal string
		signature := hex.EncodeToString(hmac.Sum(nil))
		fmt.Println("Signature: " + signature)
		payload.Signature = signature
		var jsonPayload []byte

		jsonPayload, err = json.Marshal(payload)
		if err != nil {
			log.Println(err)
		}

		var endpoint = "https://test-payment.momo.vn/v2/gateway/api/create"
		resp, err := http.Post(endpoint, "application/json", bytes.NewBuffer(jsonPayload))
		if err != nil {
			log.Fatalln(err)
		}
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			log.Fatalln(err)
		}
		fmt.Println(result)
		fmt.Println("PayUrl is: %s\n", result["payUrl"])
		res := result["payUrl"]

		_, anyErr := PayloadCollection.InsertOne(ctx, payload)
		if anyErr != nil {
			response.Status = "Failed"
			response.Code = http.StatusInternalServerError
			response.Msg = "Not created"
			c.IndentedJSON(http.StatusInternalServerError, response)
			return
		}
		defer cancel()

		response.Status = "OK"
		response.Code = http.StatusOK
		response.Msg = "New payload has been successfully added"
		response.Data = res
		c.IndentedJSON(http.StatusOK, response)

		return
	}
}

func GetAllPayloads() gin.HandlerFunc {
	return func(c *gin.Context) {
		var response models.Response
		var payloadList []models.Payload

		var ctx, cancel = context.WithTimeout(context.Background(), 100*time.Second)
		defer cancel()

		cursor, err := PayloadCollection.Find(ctx, bson.D{{}})
		if err != nil {
			response.Status = "Failed"
			response.Code = http.StatusInternalServerError
			response.Msg = "Something went wrong. Please try again later"
			c.IndentedJSON(http.StatusInternalServerError, response)
			return
		}
		err = cursor.All(ctx, &payloadList)
		if err != nil {
			log.Println(err)
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		defer cursor.Close(ctx)
		if err := cursor.Err(); err != nil {
			log.Println(err)
			response.Status = "Failed"
			response.Code = 400
			response.Msg = "Invalid"
			c.IndentedJSON(400, response)
			return
		}

		response.Status = "OK"
		response.Code = 200
		response.Msg = "Successfully"
		response.Data = payloadList
		c.IndentedJSON(200, response)
		return
	}
}
