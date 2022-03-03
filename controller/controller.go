package controller

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/brave/go-translate/language"
	"github.com/brave/go-translate/translate"
	"github.com/go-chi/chi"
	log "github.com/sirupsen/logrus"
)

// LnxEndpoint specifies the remote Lnx translate server used by
// brave-core, and it can be set to a mock server during testing.
var LnxEndpoint = os.Getenv("LNX_HOST")
var LnxPort = os.Getenv("LNX_PORT")
var LnxApiKey = os.Getenv("LNX_API_KEY")

// GoogleTranslateServerProxy specifies the proxy server for requesting
// resource from google translate server, and it can be set to a mock server
// during testing.
var GoogleTranslateServerProxy = "https://translate.brave.com"

const (
	// GoogleTranslateServer specifies the remote google translate server.
	GoogleTranslateServer = "https://translate.googleapis.com"

	// GStaticServerProxy specifies the proxy server for requesting resource
	// from google gstatic server.
	GStaticServerProxy = "https://translate-static.brave.com"

	languagePath  = "/get-languages"
	translatePath = "/translate"
)

// TranslateRouter add routers for translate requests and translate script
// requests.
func TranslateRouter() chi.Router {
	r := chi.NewRouter()

	r.Post("/translate", Translate)
	r.Options("/translate", HandleCorsPreflight)
	r.Get("/language", GetLanguageList)

	return r
}

func HandleCorsPreflight(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
}

func getHTTPClient() *http.Client {
	return &http.Client{
		Timeout: time.Second * 10,
	}
}

// GetLanguageList send a request to Lingvanex server and convert the response
// into google format and reply back to the client.
func GetLanguageList(w http.ResponseWriter, r *http.Request) {
	// Send a get language list request to Lnx
	req, err := http.NewRequest("GET", LnxEndpoint+languagePath+":"+LnxPort, nil)
	req.Header.Add("Authorization", "Bearer "+LnxApiKey)

	if err != nil {
		http.Error(w, fmt.Sprintf("Error creating Lnx request: %v", err), http.StatusInternalServerError)
		return
	}

	client := getHTTPClient()
	lnxResp, err := client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error sending request to Lnx server: %v", err), http.StatusInternalServerError)
		return
	}
	defer func() {
		err := lnxResp.Body.Close()
		if err != nil {
			log.Errorf("Error closing response body stream: %v", err)
		}
	}()

	// Set response header
	w.Header().Set("Content-Type", lnxResp.Header["Content-Type"][0])
	w.WriteHeader(lnxResp.StatusCode)

	// Copy resonse body if status is not OK
	if lnxResp.StatusCode != http.StatusOK {
		_, err = io.Copy(w, lnxResp.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error copying Lnx response body: %v", err), http.StatusInternalServerError)
		}
		return
	}

	// Convert to google format language list and write it back
	lnxBody, err := ioutil.ReadAll(lnxResp.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading Lnx response body: %v", err), http.StatusInternalServerError)
		return
	}
	body, err := language.ToGoogleLanguageList(lnxBody)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error converting to google language list: %v", err), http.StatusInternalServerError)
		return
	}
	_, err = w.Write(body)
	if err != nil {
		log.Errorf("Error writing response body for translate requests: %v", err)
	}
}

// Translate converts a Google format translate request into a Lingvanex format
// one which will be send to the Lingvanex server, and write a Google format
// response back to the client.
func Translate(w http.ResponseWriter, r *http.Request) {
	req, isAuto, err := translate.ToLingvanexRequest(r, LnxEndpoint+translatePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error converting to LnxEndpoint request: %v", err), http.StatusBadRequest)
		return
	}

	req.Header.Add("Authorization", "Bearer "+LnxApiKey)

	// Send translate request to Lnx server
	client := getHTTPClient()
	lnxResp, err := client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error sending request to LnxEndpoint: %v", err), http.StatusInternalServerError)
		return
	}
	defer func() {
		err := lnxResp.Body.Close()
		if err != nil {
			log.Errorf("Error closing response body stream: %v", err)
		}
	}()

	// Set Header
	w.Header().Set("Content-Type", lnxResp.Header["Content-Type"][0])
	w.Header().Set("Access-Control-Allow-Origin", "*") // same as Google response

	// Copy resonse body if status is not OK
	if lnxResp.StatusCode != http.StatusOK {
		w.WriteHeader(lnxResp.StatusCode)
		_, err = io.Copy(w, lnxResp.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error copying LnxEndpoint response body: %v", err), http.StatusInternalServerError)
		}
		return
	}

	// Set google format response body
	lnxBody, err := ioutil.ReadAll(lnxResp.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading LnxEndpoint response body: %v", err), http.StatusInternalServerError)
		return
	}
	body, err := translate.ToGoogleResponseBody(lnxBody, isAuto)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error converting to google response body: %v", err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(lnxResp.StatusCode)
	_, err = w.Write(body)
	if err != nil {
		log.Errorf("Error writing response body for translate requests: %v", err)
	}
}
