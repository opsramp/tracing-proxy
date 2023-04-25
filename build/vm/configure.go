package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

func main() {
	configFile, err := os.ReadFile("/opt/opsramp/tracing-proxy/conf/config_complete.yaml")
	if err != nil {
		log.Fatal(err)
	}

	api := flag.String("A", "", "API for Authorization")
	key := flag.String("K", "", "OpsRamp Key")
	secret := flag.String("S", "", "OpsRamp Secret")
	tenant := flag.String("T", "", "OpsRamp TenantID")
	tracesAPI := flag.String("B", "", "API to Sent Traces (Defaults to Authorization API specified using -A flag if not set)")
	metricsAPI := flag.String("M", "", "API To Send Metrics (Defaults to Authorization API specified using -A flag if not set)")
	flag.Parse()

	if *api == "" {
		log.Fatal("api cant be empty, please specify using -A flag")
	}
	if *key == "" {
		log.Fatal("key cant be empty, please specify using -K flag")
	}
	if *secret == "" {
		log.Fatal("secret cant be empty, please specify using -S flag")
	}
	if *tenant == "" {
		log.Fatal("tenant cant be empty, please specify using -T flag")
	}
	if *tracesAPI == "" {
		*tracesAPI = *api
	}
	if *metricsAPI == "" {
		*metricsAPI = *api
	}

	fileContent := string(configFile)
	fileContent = strings.ReplaceAll(fileContent, "<OPSRAMP_API>", *api)
	fileContent = strings.ReplaceAll(fileContent, "<OPSRAMP_TRACES_API>", *tracesAPI)
	fileContent = strings.ReplaceAll(fileContent, "<OPSRAMP_METRICS_API>", *metricsAPI)
	fileContent = strings.ReplaceAll(fileContent, "<KEY>", *key)
	fileContent = strings.ReplaceAll(fileContent, "<SECRET>", *secret)
	fileContent = strings.ReplaceAll(fileContent, "<TENANT_ID>", *tenant)

	if err = os.WriteFile("/opt/opsramp/tracing-proxy/conf/config_complete.yaml", []byte(fileContent), 600); err != nil {
		log.Fatal(err)
	}

	if _, err := exec.Command("systemctl", "enable", "--now", "tracing-proxy").Output(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Tracing-Proxy Started Successfully")
}
