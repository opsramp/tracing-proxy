package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	var configFile []byte
	var fileContent string
	var err error

	configFile, err = os.ReadFile("/opt/opsramp/tracing-proxy/conf/config_complete.yaml")

	api := flag.String("A", "", "API To Send Data")
	key := flag.String("K", "", "Opsramp Key")
	secret := flag.String("S", "", "Opsramp Secret")
	tenant := flag.String("T", "", "Opsramp TenantID")
	metricsAPI := flag.String("M", "", "API To Send Metrics Data")
	flag.Parse()

	fileContent = string(configFile)
	fileContent = strings.ReplaceAll(fileContent, "<OPSRAMPAPI>", *api)
	fileContent = strings.ReplaceAll(fileContent, "<OPSRAMPMETRICSAPI>", *metricsAPI)
	fileContent = strings.ReplaceAll(fileContent, "<KEY>", *key)
	fileContent = strings.ReplaceAll(fileContent, "<SECRET>", *secret)
	fileContent = strings.ReplaceAll(fileContent, "<TENANTID>", *tenant)

	if err = os.WriteFile("/opt/opsramp/tracing-proxy/conf/config_complete.yaml", []byte(fileContent), 0666); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if _, err := exec.Command("systemctl", "enable", "--now", "tracing-proxy").Output(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println("Tracing-Proxy Started Successfully")
}
