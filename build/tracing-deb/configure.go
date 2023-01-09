package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
)

func main() {
	var configFile, updatedConfigFile []byte
	var err error
	configFile, err = ioutil.ReadFile("/opt/opsramp/tracing-proxy/conf/config_complete.toml")

	api := flag.String("A", "", "API To Send Data")
	key := flag.String("K", "", "Opsramp Key")
	secret := flag.String("S", "", "Opsramp Secret")
	tenant := flag.String("T", "", "Opsramp TenantID")
	flag.Parse()

	opsrampApiHost := "OpsrampAPI = \"" + *api + "\""
	opsrampMetricsApiHost := "OpsRampMetricsAPI = \"" + *api + "\""

	updatedConfigFile = bytes.Replace(configFile, []byte("OpsrampAPI = "), []byte(opsrampApiHost), 1)
	updatedConfigFile = bytes.Replace(updatedConfigFile, []byte("OpsRampMetricsAPI = "), []byte(opsrampMetricsApiHost), 1)

	opsrampKey := "OpsrampKey = \"" + *key + "\""
	opsrampMetricsApiKey := "OpsRampMetricsAPIKey = \"" + *key + "\""

	updatedConfigFile = bytes.Replace(updatedConfigFile, []byte("OpsrampKey = "), []byte(opsrampKey), 1)
	updatedConfigFile = bytes.Replace(updatedConfigFile, []byte("OpsRampMetricsAPIKey = "), []byte(opsrampMetricsApiKey), 1)

	OpsrampSecret := "OpsrampSecret = \"" + *secret + "\""
	OpsRampMetricsAPISecret := "OpsRampMetricsAPISecret = \"" + *secret + "\""

	updatedConfigFile = bytes.Replace(updatedConfigFile, []byte("OpsrampSecret = "), []byte(OpsrampSecret), 1)
	updatedConfigFile = bytes.Replace(updatedConfigFile, []byte("OpsRampMetricsAPISecret = "), []byte(OpsRampMetricsAPISecret), 1)

	opsrampTenantID := "OpsRampTenantID = \"" + *tenant + "\""
	TenantId := "TenantId = \"" + *tenant + "\""

	updatedConfigFile = bytes.Replace(updatedConfigFile, []byte("OpsRampTenantID = "), []byte(opsrampTenantID), 1)
	updatedConfigFile = bytes.Replace(updatedConfigFile, []byte("TenantId = "), []byte(TenantId), 1)

	if err = ioutil.WriteFile("/opt/opsramp/tracing-proxy/conf/config_complete.toml", updatedConfigFile, 0666); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if _, err := exec.Command("systemctl", "start", "tracing-proxy").Output(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("Tracing-Proxy Started Successfully")
}
