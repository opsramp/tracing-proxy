package main

import (
	"flag"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

const ServiceName = "tracing-proxy"

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

	systemctl := exec.Command("systemctl", "--version").Run()
	if systemctl == nil { //checking os type here

		_ = exec.Command("cp", "/opt/opsramp/service_files/tracing-proxy.service", "/etc/systemd/system/tracing-proxy.service").Run()
		_ = exec.Command("chmod", "0644", "/etc/systemd/system/tracing-proxy").Run()
		_ = exec.Command("rm", "-rf", "/opt/opsramp/service_files").Run()

		// Enable and start with fallback
		if err := exec.Command("systemctl", "enable", "--now", ServiceName).Run(); err != nil {
			_ = exec.Command("systemctl", "start", ServiceName).Run()
			_ = exec.Command("systemctl", "enable", ServiceName).Run()
		}
	} else {
		_ = exec.Command("mv", "/opt/opsramp/service_files/tracing-proxy", "/etc/init.d/tracing-proxy").Run()
		_ = exec.Command("chmod", "0755", "/etc/init.d/tracing-proxy").Run()
		_ = exec.Command("rm", "-rf", "/opt/opsramp/service_files").Run()
	}

	time.Sleep(5 * time.Second)

	systemctl = exec.Command("systemctl", "--version").Run()
	if systemctl == nil {
		//Check if the services are enabled and started properly and attempt again
		if output, err := exec.Command("systemctl", "is-enabled", ServiceName).Output(); err != nil || string(output) != "enabled" {
			_ = exec.Command("service", ServiceName, "enable").Run()
		}
		if output, err := exec.Command("systemctl", "is-active", ServiceName).Output(); err != nil || string(output) != "active" {
			_ = exec.Command("service", ServiceName, "start").Run()
		} else {
			log.Println("Tracing-Proxy Started Successfully")
		}
	} else {
		if err := exec.Command("service", ServiceName, "start").Run(); err != nil {
			log.Fatal(err)
		}
		_ = exec.Command("chkconfig", ServiceName, "on")
		log.Println("Tracing-Proxy Started successfully")
	}
}
