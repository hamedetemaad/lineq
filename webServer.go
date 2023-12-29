package main

import (
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"strings"

	"github.com/gorilla/websocket"
)

var users = make(map[chan string]bool)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type ResponseBody struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type ConfigResponse struct {
	Status               string `json:"status"`
	Message              string `json:"message"`
	RoomTableName        string `json:"lineq_room_table"`
	UserTableName        string `json:"lineq_user_table"`
	LineqSessionDuration int    `json:"lineq_session_duration"`
}

type RequestBody struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Host        string `json:"host"`
	ActiveUsers int    `json:"activeUsers"`
}

type WebClient struct {
	conn *websocket.Conn
}

var webClients []*WebClient

func initWebServer(web_host string, web_port string) {
	http.HandleFunc("/", handleWebRequests)
	http.HandleFunc("/tables", getTables)
	http.HandleFunc("/getConfig", getConfig)
	http.HandleFunc("/create", createTables)
	http.HandleFunc("/ws", handleWebSocket)
	addr := web_host + ":" + web_port
	log.Println("Server is running on ", addr)
	http.ListenAndServe(addr, nil)
}

func handleWebRequests(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Accept") == "text/event-stream" {
		cookie := r.URL.Query().Get("info")
		hostname := r.URL.Query().Get("host")
		pathname := r.URL.Query().Get("path")
		log.Println(cookie, hostname, pathname)
		handleSSE(w, r, cookie, hostname, pathname)
	} else if strings.Contains(r.URL.Path, "/lineq/") {
		parts := strings.SplitN(r.URL.Path, "/lineq/", 2)
		if len(parts) > 1 {
			newPath := "/static/lineq/" + parts[1]
			http.ServeFile(w, r, "."+newPath)
			return
		}
		http.ServeFile(w, r, "./static/index.html")
	} else {
		http.ServeFile(w, r, "./static/index.html")
	}
}

func handleSSE(w http.ResponseWriter, r *http.Request, cookie string, hostname string, pathname string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	messageChan := make(chan string)

	users[messageChan] = true

	notify := w.(http.CloseNotifier).CloseNotify()
	go func() {
		<-notify
		delete(users, messageChan)
		close(messageChan)
	}()

	initialQueue := getQueue(cookie, hostname, pathname)
	fmt.Fprintf(w, "data: %s\n\n", initialQueue)
	w.(http.Flusher).Flush()

	for message := range messageChan {
		fmt.Fprintf(w, "data: %s\n\n", message)
		w.(http.Flusher).Flush()
	}
}

func getQueue(cookie string, hostname string, pathname string) string {

	domain := strings.Replace(hostname, ".", "_", -1)
	path := strings.Replace(pathname, "/", "_", -1)
	name := fmt.Sprintf("%s%s", domain, path)

	sid := strings.Split(cookie, "=")[1]
	id := fmt.Sprintf("%s@%s", sid, name)
	log.Println(name, id)
	key := []byte(id)
	jsonKey, _ := json.Marshal(&key)
	keyEnc := b64.StdEncoding.EncodeToString(jsonKey)
	j := -1
	for i := 0; i < len(sortedEntries[name]); i++ {
		if sortedEntries[name][i] == keyEnc {
			j = i
			break
		}
	}
	return fmt.Sprintf("%d", j+1)
}

func broadcast() {
	for user := range users {
		user <- "DEC"
	}
}

func getTables(w http.ResponseWriter, r *http.Request) {
	messageJSON := parseTables()
	w.Header().Set("Content-Type", "application/json")

	_, err := w.Write(messageJSON)
	if err != nil {
		log.Println("Error writing JSON response:", err)
	}
}

func getConfig(w http.ResponseWriter, r *http.Request) {
	response := ConfigResponse{
		Status:               "OK",
		Message:              "Config Retrieved Successfully",
		RoomTableName:        service_vwr_room_table,
		UserTableName:        service_vwr_user_table,
		LineqSessionDuration: service_vwr_session_duration,
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	w.Write(jsonResponse)
}

func createTables(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var requestBody RequestBody
	err := json.NewDecoder(r.Body).Decode(&requestBody)
	if err != nil {
		http.Error(w, "Error decoding JSON request body", http.StatusBadRequest)
		return
	}

	name := requestBody.Name
	path := requestBody.Path
	host := requestBody.Host
	activeUsers := requestBody.ActiveUsers
	updateRoomTable(name, path, host, activeUsers)

	response := ResponseBody{
		Status:  "success",
		Message: fmt.Sprintf("Tables Updated"),
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		http.Error(w, "Error encoding JSON response", http.StatusInternalServerError)
		return
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
