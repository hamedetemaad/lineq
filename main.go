package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"

	b64 "encoding/base64"
	"encoding/json"
)

var peers []*Client
var sortedEntries []string
var tables = make(map[string]Table)

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}

func main() {
	initLogger()

	service_tcp_host := getenv("SERVICE_TCP_HOST", DEFAULT_TCP_HOST)
	service_tcp_port := getenv("SERVICE_TCP_PORT", DEFAULT_TCP_PORT)
	service_web_host := getenv("SERVICE_WEB_HOST", DEFAULT_WEB_HOST)
	service_web_port := getenv("SERVICE_WEB_PORT", DEFAULT_WEB_PORT)
	service_mode := getenv("SERVICE_MODE", DEFAULT_MODE) // vwr(virtual waiting room) or agg(Aggregation) or acc(Accumulation)
	service_vwr_session_duration := getenv("SERVICE_VWR_SESSION_DURATION", DEFAULT_VWR_SESSION_DURATION)
	service_vwr_total_users := getenv("SERVICE_VWR_TOTAL_ACTIVE_USERS", DEFAULT_VWR_TOTAL_USERS)
	service_vwr_room_table := getenv("SERVICE_VWR_ROOM_TABLE", DEFAULT_VWR_ROOM_TABLE)
	service_vwr_users_table := getenv("SERVICE_VWR_USERS_TABLE", DEFAULT_VWR_USERS_TABLE)
	vwr_session_duration, _ := strconv.Atoi(service_vwr_session_duration)
	vwr_total_users, _ := strconv.Atoi(service_vwr_total_users)

	cFlag := flag.Bool("c", false, "generate haproxy configuration (boolean)")
	flag.Parse()

	go initWebServer(service_web_host, service_web_port)
	listen, err := net.Listen("tcp", service_tcp_host+":"+service_tcp_port)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	if service_mode == "vwr" {
		if *cFlag {
			generateHAProxyConfiguration(service_vwr_room_table, service_vwr_users_table, vwr_session_duration, service_web_host, service_web_port, service_tcp_host, service_tcp_port)
			os.Exit(0)
		}

		initRoomTable(vwr_total_users, service_vwr_room_table)
		go initCache(vwr_session_duration, vwr_total_users, service_vwr_room_table, service_vwr_users_table)
	}

	defer listen.Close()
	for {
		conn, err := listen.Accept()
		if err != nil {
			log.Fatal(err)
			os.Exit(1)
		}

		client := &Client{
			conn:   conn,
			reader: bufio.NewReader(conn),
		}
		peers = append(peers, client)
		go client.initConnection(service_mode, vwr_session_duration, vwr_total_users, service_vwr_room_table, service_vwr_users_table)
	}
}

func initRoomTable(vwr_total_users int, roomName string) {
	frequency := [][]int{}
	dType := []int{GPC0}
	tableDefinition := TableDefinition{
		StickTableID: 777,
		Name:         roomName,
		KeyType:      SINT,
		KeyLen:       4,
		DataTypes:    dType,
		Expiry:       24 * 60 * 60 * 1000,
		Frequency:    frequency,
	}

	roomTable := Table{
		localUpdateId: 0,
		definition:    tableDefinition,
	}

	var key []byte = s32tob(1)

	jsonKey, _ := json.Marshal(&key)
	keyEnc := b64.StdEncoding.EncodeToString(jsonKey)

	roomTable.entries = make(map[string]Entry)
	var entryKey int32 = 1
	roomEntry := Entry{
		Key: entryKey,
	}
	roomEntry.Values = make(map[int][]int)
	roomEntry.Values[GPC0] = []int{vwr_total_users}
	roomTable.entries[keyEnc] = roomEntry
	tables[roomName] = roomTable
	sortedEntries = make([]string, 0)
}

func initLogger() {
	log.SetFlags(log.Ldate | log.Ltime)
	log.SetPrefix("lineQ   ")
}

func generateHAProxyConfiguration(roomTable string, usersTable string, session_duration int, webHost string, webPort string, tcpHost string, tcpPort string) {
	fileName := "haproxy.cfg"
	config := ""
	file, err := os.Create(fileName)

	if err != nil {
		log.Println("Error creating the file:", err)
		return
	}
	defer file.Close() // Ensure the file is closed when done

	config += fmt.Sprintln("#### (lineQ) HAProxy BASIC Configuration ####")
	config += fmt.Sprintln("#### (lineQ) please adjust this config according to your needs ####")
	config += fmt.Sprintln("peers lineq")
	config += fmt.Sprintln("\tbind 0.0.0.0:55555")
	config += fmt.Sprintln("\tserver haproxy1")
	config += fmt.Sprintf("\tserver lineq %s:%s\n", tcpHost, tcpPort)
	config += fmt.Sprintf("backend %s\n", roomTable)
	config += fmt.Sprintf("\tstick-table type integer size 2 expire 1d store gpc0 peers lineq\n")
	config += fmt.Sprintf("backend %s\n", usersTable)
	config += fmt.Sprintf("\tstick-table type string len 36 size 100k expire %vm store gpc1 peers lineq\n", session_duration)
	config += fmt.Sprintln("frontend fe_main")
	config += fmt.Sprintf("\tbind *:80\n")
	config += fmt.Sprintf("\thttp-request track-sc0 int(1) table %s\n", roomTable)
	config += fmt.Sprintf("\thttp-request set-var(txn.has_cookie) req.cook_cnt(sessionid)\n")
	config += fmt.Sprintf("\thttp-request set-var(txn.t2) uuid()  if !{ var(txn.has_cookie) -m int gt 0 }\n")
	config += fmt.Sprintf("\thttp-response add-header Set-Cookie \"sessionid=%%[var(txn.t2)]; path=/\" if !{ var(txn.has_cookie) -m int gt 0 }\n")
	config += fmt.Sprintf("\thttp-request set-var(txn.sessionid) req.cook(sessionid)\n")
	config += fmt.Sprintf("\thttp-request track-sc1 var(txn.sessionid) table timestamps if { var(txn.has_cookie) -m int gt 0 }\n")
	config += fmt.Sprintf("\thttp-request track-sc1 var(txn.t2) table timestamps if !{ var(txn.has_cookie) -m int gt 0 }\n")
	config += fmt.Sprintf("\tacl has_slot sc_get_gpc1(1) eq 1\n")
	config += fmt.Sprintf("\tacl free_slot sc_get_gpc0(0) gt 0\n")
	config += fmt.Sprintf("\thttp-request sc-inc-gpc1(1) if free_slot !has_slot\n")
	config += fmt.Sprintf("\tuse_backend bk_yes if has_slot\n")
	config += fmt.Sprintf("\tdefault_backend bk_no\n")
	config += fmt.Sprintln("backend bk_yes")
	config += fmt.Sprintln("\tmode http")
	config += fmt.Sprintln("\t#### (lineq) change to actual ip:port(s) of your service")
	config += fmt.Sprintln("\tserver server 127.0.0.1:8889")
	config += fmt.Sprintln("backend bk_no")
	config += fmt.Sprintln("\tmode http")
	config += fmt.Sprintf("\tserver lineq %s:%s\n", webHost, webPort)

	data := []byte(config)
	_, err = file.Write(data)
	if err != nil {
		fmt.Println("Error writing to the file:", err)
		return
	}

	fmt.Println("Data has been written to", fileName)
}
