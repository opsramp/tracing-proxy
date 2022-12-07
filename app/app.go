package app

import (
	"encoding/json"
	"fmt"
	"github.com/jirs5/tracing-proxy/collect"
	"github.com/jirs5/tracing-proxy/config"
	"github.com/jirs5/tracing-proxy/logger"
	"github.com/jirs5/tracing-proxy/metrics"
	"github.com/jirs5/tracing-proxy/route"
	"io/ioutil"
	"net/http"
	"strings"
)


var OpsrampToken string
type App struct {
	Config         config.Config     `inject:""`
	Logger         logger.Logger     `inject:""`
	IncomingRouter route.Router      `inject:"inline"`
	PeerRouter     route.Router      `inject:"inline"`
	Collector      collect.Collector `inject:""`
	Metrics        metrics.Metrics   `inject:"metrics"`
	Client         http.Client
	// Version is the build ID for tracing-proxy so that the running process may answer
	// requests for the version
	Version string
}

type OpsRampAuthTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64    `json:"expires_in"`
	Scope       string `json:"scope"`
}



// Start on the App obect should block until the proxy is shutting down. After
// Start exits, Stop will be called on all dependencies then on App then the
// program will exit.
func (a *App) Start() error {
	a.Logger.Debug().Logf("Starting up App...")
	OpsrampToken = a.opsrampOauthToken()
	a.IncomingRouter.SetVersion(a.Version)
	a.PeerRouter.SetVersion(a.Version)

	// launch our main routers to listen for incoming event traffic from both peers
	// and external sources
	a.IncomingRouter.LnS("incoming")
	a.PeerRouter.LnS("peer")

	return nil
}

func (a *App) Stop() error {
	a.Logger.Debug().Logf("Shutting down App...")
	return nil
}

func (a *App) opsrampOauthToken() string  {
	OpsrampKey, _ := a.Config.GetOpsrampKey()
	OpsrampSecret, _ := a.Config.GetOpsrampSecret()
	ApiEndPoint, _ := a.Config.GetOpsrampAPI()

	fmt.Println("OpsrampKey:", OpsrampKey)
	fmt.Println("OpsrampSecret:", OpsrampSecret)
	url := fmt.Sprintf("%s/auth/oauth/token", strings.TrimRight(ApiEndPoint, "/"))
	fmt.Println(url)
	requestBody := strings.NewReader("client_id=" + OpsrampKey + "&client_secret=" + OpsrampSecret + "&grant_type=client_credentials")
	req, err := http.NewRequest(http.MethodPost, url, requestBody)
	fmt.Println("request error is: ", err)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Accept", "application/json")
	req.Header.Set("Connection", "close")

	resp, err := a.Client.Do(req)
	defer resp.Body.Close()
	fmt.Println("Response error is: ", err)

	respBody, err := ioutil.ReadAll(resp.Body)
	fmt.Println("resp.Body is ", string(respBody))
	var tokenResponse OpsRampAuthTokenResponse
	err = json.Unmarshal(respBody, &tokenResponse)
	//fmt.Println("tokenResponse", tokenResponse)
	fmt.Println("tokenResponse.AccessToken: ", tokenResponse.AccessToken)
	return tokenResponse.AccessToken
}

