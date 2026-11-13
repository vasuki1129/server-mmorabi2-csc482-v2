package main

import (
	"net/http"
	"encoding/json"
	"log"
	"time"
	"os"
	"strings"
	"fmt"
	"strconv"
	
)

import (
	"context"                    
	                        

	"github.com/aws/aws-sdk-go-v2/aws"               
	"github.com/aws/aws-sdk-go-v2/config"            
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue" 
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var loggly_url string
var client *dynamodb.Client

type LogLevel int

const (
	Info LogLevel = iota
	Warn
	Error
)


type DBOrder struct {
	IsBuyOrder bool `dynamodbav:"IsBuyOrder"`
	LocationId int `dynamodbav:"LocationId"`
	Price float32 `dynamodbav:"Price"`
	TypeId int `dynamodbav:"TypeId"`
	VolumeRemain int `dynamodbav:"VolumeRemain"`
	VolumeTotal int `dynamodbav:"VolumeTotal"`
}

type DBRecord struct {
	TypeId int `dynamodbav:"TypeId"`
	Timestamp int64 `dynamodbav:"Timestamp"`
	OrderBook []DBOrder `dynamodbav:"OrderBook"`
}


type StatusResponse struct {
	SystemTime string `json:"system_time"`
}

func log_loggly(str string, req *http.Request, log_level LogLevel){
	
	body := strings.NewReader(str)

	if req != nil {
		body = strings.NewReader(fmt.Sprintf("%s :: %s :: %s :: %s", req.RemoteAddr, req.Method, req.URL, str))
	}
	
	req, err := http.NewRequest("POST", loggly_url, body)
	if err != nil {
		panic("Could not create logging request")
	}

	if log_level == Info {
		req.Header.Set("X-LOGGLY-TAG","Info")
	} else if log_level == Warn {
		req.Header.Set("X-LOGGLY-TAG","Warning")
	} else if log_level == Error {
		req.Header.Set("X-LOGGLY-TAG","Error")
	} else {
		fmt.Printf("Incorrect logging level supplied to log_loggly()\n")
		return
	}

	client := &http.Client {
		Timeout: 10 * time.Second,
		}

	resp,err := client.Do(req)
	if err != nil {
		fmt.Println("Error, could not send log data to Loggly")
		return
	}
	resp.Body.Close()
}


// WARNING: I really don't think this is a good idea this will eat up all
//the dynamodb pay-me-money units if the table is large.  It p a g e s it
//will actually send it all
func allHandler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	p := dynamodb.NewScanPaginator(client, &dynamodb.ScanInput{
		TableName: aws.String("mmorabi2_EveOrderTable"),
	})

	var items []map[string]types.AttributeValue
	for p.HasMorePages() {
		out, err := p.NextPage(ctx)
		if err != nil {

			http.Error(w,err.Error(), http.StatusInternalServerError)
			return
		}
		items = append(items, out.Items...)
	}

	var result []DBRecord
	err := attributevalue.UnmarshalListOfMaps(items, &result)
	
	marshal_err := json.NewEncoder(w).Encode(result)
	if (marshal_err != nil) || (err != nil) {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}


func searchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		log_loggly("Incorrect method sent to web server :: 405", r, Error)
		return
	}

	typeid_quer := strings.TrimSpace(r.URL.Query().Get("typeid"))
	type_id, err := strconv.ParseInt(typeid_quer, 10, 64)
	if err != nil {
		http.Error(w, "Missing or malformed typeid", http.StatusBadRequest)
		log_loggly("Missing or malformed typeid :: 400", r, Error)
		return
	}


	key_cond := "TypeId = :typeid"
	attr_names := map[string]string{}
	attr_values := map[string]types.AttributeValue{
		":typeid": &types.AttributeValueMemberN{Value: strconv.FormatInt(type_id, 10)},
	}

	// Timestamp is the sort key; if no timestamp is given, return everything for the TypeId.
	timestamp_quer := strings.TrimSpace(r.URL.Query().Get("timestamp"))
	if timestamp_quer != "" {
		quer_dat := parseTimestampQuery(timestamp_quer)
		if quer_dat.QueryValueA < 0 || (quer_dat.Type == Type_Range && quer_dat.QueryValueB < 0) {
			quer_dat.Type = Type_Error
		}

		attr_names["#ts"] = "Timestamp"
		attr_values[":a"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(quer_dat.QueryValueA, 10)}

		switch quer_dat.Type {
		case Type_Exact:
			key_cond += " AND #ts = :a"
		case Type_LessThan:
			key_cond += " AND #ts < :a"
		case Type_GreatThan:
			key_cond += " AND #ts > :a"
		case Type_Range:
			key_cond += " AND #ts BETWEEN :a AND :b"
			attr_values[":b"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(quer_dat.QueryValueB, 10)}
		default:
			http.Error(w, "Query string malformed", http.StatusBadRequest)
			log_loggly("Malformed timestamp query :: 400", r, Error)
			return
		}
	}

	input := &dynamodb.QueryInput{
		TableName:                 aws.String("mmorabi2_EveOrderTable"),
		KeyConditionExpression:    aws.String(key_cond),
		ExpressionAttributeValues: attr_values,
	}
	if len(attr_names) > 0 {
		input.ExpressionAttributeNames = attr_names
	}

	ctx := r.Context()
	p := dynamodb.NewQueryPaginator(client, input)

	var items []map[string]types.AttributeValue
	for p.HasMorePages() {
		out, err := p.NextPage(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log_loggly(fmt.Sprintf("DynamoDB query failed: %v :: 500", err), r, Error)
			return
		}
		items = append(items, out.Items...)
	}

	result := []DBRecord{}
	err = attributevalue.UnmarshalListOfMaps(items, &result)
	if err != nil {
		http.Error(w, "Failed to unmarshal records", http.StatusInternalServerError)
		log_loggly("Failed to unmarshal DynamoDB records :: 500", r, Error)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(result)
	if err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		log_loggly("Failed to encode JSON response :: 500", r, Error)
		return
	}
	log_loggly(fmt.Sprintf("Successfully sent %d search results :: 200", len(result)), r, Info)
}


func notFoundHandler(w http.ResponseWriter, r *http.Request) {

	log_loggly("Page not found",r,Error)
	http.Error(w, "404 file not found", http.StatusNotFound)
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		log_loggly("Incorrect method sent to web server :: 405",r,Error)
		return
	}
	resp := StatusResponse{SystemTime: time.Now().Format(time.RFC3339)}
	w.Header().Add("Content-Type", "application/json")
	
	err := json.NewEncoder(w).Encode(resp);
	if err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		log_loggly("Failed to encode JSON response :: 500",r,Error)
		return
	}
	log_loggly("Successfully sent status message :: 200",r, Info)
}

func main() {
	loggly_url = os.Getenv("LOGGLY_URL")

	ctx := context.Background()
	
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}

	// 2. Initialize the DynamoDB client
	client = dynamodb.NewFromConfig(cfg)

	mux := http.NewServeMux()
	
	mux.HandleFunc("/mmorabi2/status", statusHandler)

	mux.HandleFunc("/mmorabi2/all", allHandler)
	mux.HandleFunc("/mmorabi2/search", searchHandler)

	mux.HandleFunc("/", notFoundHandler)

	log.Println("Starting server on :8080...")
	serve_err := http.ListenAndServe(":8080", mux)
	if serve_err != nil {
		log.Fatalf("Server failed to start :: %v", err)
		log_loggly("Web server failed to start",nil,Error)
	}
}






