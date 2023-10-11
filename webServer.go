package main

import (
	"encoding/json"
	"fmt"
	"log"
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
	log.Println("Server is running on ", addr)
	http.ListenAndServe(addr, nil)
}

func getTables(w http.ResponseWriter, r *http.Request) {
	messageJSON := parseTables()
	w.Header().Set("Content-Type", "application/json")

	_, err := w.Write(messageJSON)
	if err != nil {
		log.Println("Error writing JSON response:", err)
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	//defer conn.Close()

	webClient := &WebClient{
		conn: conn,
	}
	webClients = append(webClients, webClient)

	log.Println("Client connected")

	messageJSON := parseTables()

	err = webClient.conn.WriteMessage(websocket.TextMessage, messageJSON)
	if err != nil {
		log.Println("WebSocket write error:", err)
		return
	}
}

func parseEntry(id string, entry Entry, keyType string, dataType []int) map[string]interface{} {
	jsonEntry := make(map[string]interface{})
	jsonEntry["id"] = id
	switch keyType {
	case "string":
		jsonEntry["key"] = entry.Key
	case "integer":
		jsonEntry["key"] = entry.Key
	case "ipv4":
		log.Println("IPv4")
		if value, ok := entry.Key.([]byte); ok {
			jsonEntry["key"] = ipToString(value, false)
		} else {
			log.Println("Conversion to []byte failed.")
			return nil
		}
	case "ipv6":
		if value, ok := entry.Key.([]byte); ok {
			jsonEntry["key"] = ipToString(value, true)
		} else {
			log.Println("Conversion to []byte failed.")
			return nil
		}
	}

	dataValues := ""
	for i := 0; i < len(dataType); i++ {
		switch dataType[i] {
		case SERVER_ID:
			dataValues += fmt.Sprintf("%d\t", entry.Values[dataType[i]][0])
		case GPT0, GPC0, CONN_CNT, CONN_CUR, SESS_CNT, HTTP_REQ_CNT, HTTP_ERR_CNT, GPC1:
			dataValues += fmt.Sprintf("%d\t", entry.Values[dataType[i]][0])
		case HTTP_REQ_RATE:
			dataValues += fmt.Sprintf("%d\t", entry.Values[dataType[i]][1])
		case BYTES_IN_CNT, BYTES_OUT_CNT:
		}
	}
	jsonEntry["value"] = dataValues

	return jsonEntry
}

func parseEntries(entries map[string]Entry, keyType string, dataType []int) []interface{} {
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

func getValueTypes(vTypes []int) string {
	values := ""

	for i := 0; i < len(vTypes); i++ {
		switch vTypes[i] {
		case SERVER_ID:
			values += "server_id  "
		case GPT0:
			values += "gpt0  "
		case GPC0:
			values += "gpc0  "
		case CONN_CNT:
			values += "conn_cnt  "
		case CONN_CUR:
			values += "conn_cur  "
		case SESS_CNT:
			values += "sess_cnt  "
		case HTTP_REQ_CNT:
			values += "http_req_cnt  "
		case HTTP_ERR_CNT:
			values += "http_err_cnt  "
		case GPC1:
			values += "gpc1  "
		case HTTP_REQ_RATE:
			values += "http_req_rate  "
		case BYTES_IN_CNT:
			values += "bytes_in_cnt  "
		case BYTES_OUT_CNT:
			values += "bytes_out_cnt  "
		}
	}

	return values
}

func parseTable(table Table) map[string]interface{} {
	jsonTable := make(map[string]interface{})
	tableDef := table.definition
	dataType := tableDef.DataTypes
	jsonTable["expiry"] = tableDef.Expiry

	keyType := getKeyType(tableDef.KeyType)
	jsonTable["type"] = keyType
	vTypes := getValueTypes(tableDef.DataTypes)
	jsonTable["vtypes"] = vTypes
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
		log.Println("JSON serialization error:", err)
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
	dataType := tableDef.DataTypes
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
			log.Println("WebSocket write error:", err)
		}
	}
}
