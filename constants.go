package main

const (
	DEFAULT_TCP_HOST  = "localhost"
	DEFAULT_WEB_HOST  = "localhost"
	DEFAULT_TCP_PORT  = "11111"
	DEFAULT_WEB_PORT  = "8060"
	DEFAULT_NAME      = "aggr1"
	DEFAULT_MODE      = "agg" // or acc
	DEFAULT_AUTO_SYNC = "false"
)

const (
	SUCCEEDED          = "200"
	TRY_AGAIN          = "300"
	PROTOCOL_ERROR     = "501"
	BAD_VERSION        = "502"
	LOCAL_ID_MISMATCH  = "503"
	REMOTE_ID_MISMATCH = "504"
)

const (
	CLASS_CONTROL  = 0
	CLASS_ERROR    = 1
	CLASS_UPDATE   = 10
	CLASS_RESERVED = 255
)

const (
	SYNCHRONIZATION_REQUEST   = 0
	SYNCHRONIZATION_FINISHED  = 1
	SYNCHRONIZATION_PARTIAL   = 2
	SYNCHRONIZATION_CONFIRMED = 3
	HEARTBEAT                 = 4
)

const (
	ENTRY_UPDATE             = 128
	INCREMENTAL_ENTRY_UPDATE = 129
	STICK_TABLE_DEFINITION   = 130
	STICK_TABLE_SWITCH       = 131
	UPDATE_ACK               = 132
)

const (
	SINT   int = 2
	IPv4   int = 4
	IPv6   int = 5
	STRING int = 6
	BINARY int = 7
)

const (
	SERVER_ID      int = 0
	GPT0           int = 1
	GPC0           int = 2
	GPC0_RATE      int = 3
	CONN_CNT       int = 4
	CONN_RATE      int = 5
	CONN_CUR       int = 6
	SESS_CNT       int = 7
	SESS_RATE      int = 8
	HTTP_REQ_CNT   int = 9
	HTTP_REQ_RATE  int = 10
	HTTP_ERR_CNT   int = 11
	HTTP_ERR_RATE  int = 12
	BYTES_IN_CNT   int = 13
	BYTES_IN_RATE  int = 14
	BYTES_OUT_CNT  int = 15
	BYTES_OUT_RATE int = 16
	GPC1           int = 17
	GPC1_RATE      int = 18
)
