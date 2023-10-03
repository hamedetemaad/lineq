package main

import (
	"bufio"
	"log"
	"net"
	"os"
	"reflect"

	b64 "encoding/base64"
	"encoding/json"
	"fmt"
)

var peers []*Client
var tables = make(map[string]Table)

type Config struct {
	TCP_HOST                string           `json:"tcp_host" default:"localhost"`
	WEB_HOST                string           `json:"web_host" default:"localhost"`
	TCP_PORT                string           `json:"tcp_port" default:"11111"`
	WEB_PORT                string           `json:"web_port" default:"8060"`
	SERVICE_NAME            string           `json:"service_name" default:"lineQ"`
	SERVICE_MODE            string           `json:"service_mode" default:"agg"`
	VWR_ROOM_TABLE          string           `json:"vwr_room_table" default:"room"`
	VWR_INACTIVITY_DURATION int              `json:"vwr_inactivity_duration"`
	VWR_ROUTES              map[string]Route `json:"routes"`
}

type Route struct {
	TOTAL_ACTIVE_USERS int    `json:"vwr_active_users"`
	PATH               string `json:"vwr_users_path"`
}

func setDefaults(config *Config) {
	valueType := reflect.ValueOf(config)
	valueTypeKind := valueType.Kind()

	if valueTypeKind != reflect.Ptr || valueType.Elem().Kind() != reflect.Struct {
		fmt.Println("Input must be a pointer to a struct")
		return
	}

	valueType = valueType.Elem()
	valueTypeType := valueType.Type()

	for i := 0; i < valueType.NumField(); i++ {
		field := valueType.Field(i)
		fieldType := valueTypeType.Field(i)
		fmt.Println(fieldType)

		if field.IsZero() {
			defaultValueTag := fieldType.Tag.Get("default")

			if defaultValueTag != "" {
				switch field.Kind() {
				case reflect.Int:
					defaultIntValue := reflect.ValueOf(defaultValueTag).Convert(field.Type())
					field.Set(defaultIntValue)
				case reflect.String:
					field.SetString(defaultValueTag)
				case reflect.Bool:
					defaultBoolValue := defaultValueTag == "true" || defaultValueTag == "1"
					field.SetBool(defaultBoolValue)
				}
			}
		}
	}
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}

func main() {
	configFile, err := os.Open("/etc/lineq/lineq.cfg")
	if err != nil {
		fmt.Println("Error opening configuration file:", err)
		return
	}
	defer configFile.Close()

	var config Config
	decoder := json.NewDecoder(configFile)
	if err := decoder.Decode(&config); err != nil {
		fmt.Println("Error decoding configuration:", err)
		return
	}

	setDefaults(&config)

	service_mode := config.SERVICE_MODE
	service_vwr_room_table := config.VWR_ROOM_TABLE
	service_tcp_host := config.TCP_HOST
	service_tcp_port := config.TCP_PORT
	service_web_host := config.WEB_HOST
	service_web_port := config.WEB_PORT
	service_name := config.SERVICE_NAME
	vwr_session_duration := config.VWR_INACTIVITY_DURATION

	go initWebServer(service_web_host, service_web_port)
	listen, err := net.Listen("tcp", service_tcp_host+":"+service_tcp_port)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	if service_mode == "vwr" {
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
		go client.initConnection(service_name, service_mode, vwr_session_duration, vwr_total_users, service_vwr_room_table, service_vwr_users_table)
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
