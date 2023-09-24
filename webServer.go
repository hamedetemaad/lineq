package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type WebClient struct {
	conn *websocket.Conn
}

var webClients []*WebClient

func initWebServer(web_host string, web_port string) {

	http.Handle("/", http.FileServer(http.Dir("./static")))
	http.HandleFunc("/tables", getTables)
	http.HandleFunc("/ws", handleWebSocket)
	addr := web_host + ":" + web_port
	fmt.Println("Server is running on ", addr)
	http.ListenAndServe(addr, nil)
}

func getTables(w http.ResponseWriter, r *http.Request) {
	messageJSON := parseTables()
	w.Header().Set("Content-Type", "application/json")

	_, err := w.Write(messageJSON)
	if err != nil {
		fmt.Println("Error writing JSON response:", err)
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	//defer conn.Close()

	webClient := &WebClient{
		conn: conn,
	}
	webClients = append(webClients, webClient)

	fmt.Println("Client connected")

	messageJSON := parseTables()

	err = webClient.conn.WriteMessage(websocket.TextMessage, messageJSON)
	if err != nil {
		fmt.Println("WebSocket write error:", err)
		return
	}
}

func parseEntry(id string, entry Entry, keyType string, dataType int) map[string]interface{} {
	jsonEntry := make(map[string]interface{})
	jsonEntry["id"] = id
	switch keyType {
	case "string":
		jsonEntry["key"] = entry.Key
	case "integer":
		jsonEntry["key"] = entry.Key
	case "ipv4":
		fmt.Println("IPv4")
		if value, ok := entry.Key.([]byte); ok {
			jsonEntry["key"] = ipToString(value, false)
		} else {
			fmt.Println("Conversion to []byte failed.")
			return nil
		}
	case "ipv6":
		if value, ok := entry.Key.([]byte); ok {
			jsonEntry["key"] = ipToString(value, true)
		} else {
			fmt.Println("Conversion to []byte failed.")
			return nil
		}
	}

	switch dataType {
	case SERVER_ID:
		jsonEntry["value"] = entry.Values[0]
	case GPT0, GPC0, CONN_CNT, CONN_CUR, SESS_CNT, HTTP_REQ_CNT, HTTP_ERR_CNT, GPC1:
		jsonEntry["value"] = entry.Values[0]
	case HTTP_REQ_RATE:
		jsonEntry["value"] = entry.Values[1]
	case BYTES_IN_CNT, BYTES_OUT_CNT:
	}

	return jsonEntry
}

func parseEntries(entries map[string]Entry, keyType string, dataType int) []interface{} {
	var jsonEntries []interface{}
	for key := range entries {
		jsonEntries = append(jsonEntries, parseEntry(key, entries[key], keyType, dataType))
	}
	return jsonEntries
}

func getKeyType(tableKeyType int) string {
	keyType := "string"

	switch tableKeyType {
	case SINT:
		keyType = "integer"
	case IPv4:
		keyType = "ipv4"
	case IPv6:
		keyType = "ipv6"
	case STRING:
		keyType = "string"
	case BINARY:
		keyType = "binary"
	}

	return keyType
}

func parseTable(table Table) map[string]interface{} {
	jsonTable := make(map[string]interface{})
	tableDef := table.definition
	dataType := tableDef.DataType
	jsonTable["expiry"] = tableDef.Expiry

	keyType := getKeyType(tableDef.KeyType)
	jsonTable["type"] = keyType
	jsonTable["entries"] = parseEntries(table.entries, keyType, dataType)
	return jsonTable
}

func parseTables() []byte {
	jsonData := make(map[string]interface{})
	jsonData["mode"] = "tables"
	for key, value := range tables {
		jsonData[key] = parseTable(value)
	}

	messageJSON, err := json.Marshal(jsonData)
	if err != nil {
		fmt.Println("JSON serialization error:", err)
		return nil
	}
	return messageJSON
}

func sendTableUpdate(tableName string, id string) {
	if len(webClients) == 0 {
		return
	}

	jsonData := make(map[string]interface{})
	jsonData["mode"] = "update"

	table := tables[tableName]
	tableDef := table.definition
	entries := table.entries
	dataType := tableDef.DataType
	keyType := getKeyType(tableDef.KeyType)

	tableInfo := make(map[string]interface{})
	tableInfo["expiry"] = tableDef.Expiry
	tableInfo["type"] = tableDef.KeyType
	tableInfo["entry"] = parseEntry(id, entries[id], keyType, dataType)
	jsonData[tableName] = tableInfo

	messageJSON, _ := json.Marshal(jsonData)

	for i := 0; i < len(webClients); i++ {
		err := webClients[i].conn.WriteMessage(websocket.TextMessage, messageJSON)
		if err != nil {
			fmt.Println("WebSocket write error:", err)
		}
	}
}
