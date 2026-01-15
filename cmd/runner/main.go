package main

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/amnezia-vpn/amneziawg-go/conn"
	"github.com/amnezia-vpn/amneziawg-go/device"
	"github.com/amnezia-vpn/amneziawg-go/tun"
	"github.com/vishvananda/netlink"
	"gopkg.in/ini.v1"
)

const (
	defaultConfigFile = "/config/wg0.conf"
	interfaceName     = "wg0"
)

func toHex(base64Str string) string {
	base64Str = strings.TrimSpace(base64Str)
	if base64Str == "" {
		return ""
	}
	keyBytes, err := base64.StdEncoding.DecodeString(base64Str)
	if err != nil {
		log.Fatalf("Failed to decode Key from Base64: %v", err)
	}
	return hex.EncodeToString(keyBytes)
}

func main() {
	configFile := os.Getenv("WG_CONFIG_FILE")
	if configFile == "" {
		configFile = defaultConfigFile
	}

	log.Printf("Starting AmneziaWG-Go Runner")
	log.Printf("Loading config from: %s", configFile)

	// Check config file permissions (should be 0600 or more restrictive)
	info, err := os.Stat(configFile)
	if err != nil {
		log.Fatalf("Failed to stat config file: %v", err)
	}
	if info.Mode().Perm()&0077 != 0 {
		log.Fatalf("Config file %s has insecure permissions %o (expected 0600 or more restrictive)", configFile, info.Mode().Perm())
	}

	// 1. Parse Config
	cfg, err := ini.Load(configFile)
	if err != nil {
		log.Fatalf("Failed to parse config file: %v", err)
	}

	// 2. Extract Interface params for UAPI and System setup
	ifaceSection := cfg.Section("Interface")
	// Extract Private Key (Precedence: Env Var > Env File > Config File)
	privateKey := os.Getenv("WG_PRIVATE_KEY")
	if privateKey == "" {
		keyFile := os.Getenv("WG_PRIVATE_KEY_FILE")
		if keyFile != "" {
			content, err := os.ReadFile(keyFile)
			if err != nil {
				log.Printf("Create warning: Could not read key file %s: %v", keyFile, err)
			} else {
				privateKey = strings.TrimSpace(string(content))
			}
		}
	}
	// Fallback to config file
	if privateKey == "" {
		privateKey = ifaceSection.Key("PrivateKey").String()
	}

	listenPort := ifaceSection.Key("ListenPort").MustInt(51820)
	fwMark := ifaceSection.Key("FwMark").MustInt(0)
	addresses := ifaceSection.Key("Address").Strings(",")
	mtu := ifaceSection.Key("MTU").MustInt(1420)

	// Amnezia Params with validation
	jc := ifaceSection.Key("Jc").MustInt(0)
	jmin := ifaceSection.Key("Jmin").MustInt(0)
	jmax := ifaceSection.Key("Jmax").MustInt(0)
	s1 := ifaceSection.Key("S1").MustInt(0)
	s2 := ifaceSection.Key("S2").MustInt(0)
	h1 := ifaceSection.Key("H1").String()
	h2 := ifaceSection.Key("H2").String()
	h3 := ifaceSection.Key("H3").String()
	h4 := ifaceSection.Key("H4").String()

	// Validate Amnezia parameters
	if jmin > 0 && jmax > 0 && jmin >= jmax {
		log.Fatalf("Invalid Amnezia config: Jmin (%d) must be less than Jmax (%d)", jmin, jmax)
	}

	// 3. Create TUN Device
	tunDev, err := tun.CreateTUN(interfaceName, mtu)
	if err != nil {
		log.Fatalf("Failed to create TUN device: %v", err)
	}
	defer tunDev.Close()

	// 4. Initialize Logger and Device (using Error level for production security)
	logger := device.NewLogger(device.LogLevelError, fmt.Sprintf("(%s) ", interfaceName))
	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), logger)
	defer dev.Close()

	err = dev.Up()
	if err != nil {
		log.Fatalf("Failed to bring up device: %v", err)
	}

	// 5. Build UAPI Config
	var uapiBuilder strings.Builder

	// Interface Config
	uapiBuilder.WriteString(fmt.Sprintf("private_key=%s\n", toHex(privateKey)))
	uapiBuilder.WriteString(fmt.Sprintf("listen_port=%d\n", listenPort))
	if fwMark != 0 {
		uapiBuilder.WriteString(fmt.Sprintf("fwmark=%d\n", fwMark))
	}

	// Amnezia Interface Config
	if jc != 0 {
		uapiBuilder.WriteString(fmt.Sprintf("jc=%d\n", jc))
	}
	if jmin != 0 {
		uapiBuilder.WriteString(fmt.Sprintf("jmin=%d\n", jmin))
	}
	if jmax != 0 {
		uapiBuilder.WriteString(fmt.Sprintf("jmax=%d\n", jmax))
	}
	if s1 != 0 {
		uapiBuilder.WriteString(fmt.Sprintf("s1=%d\n", s1))
	}
	if s2 != 0 {
		uapiBuilder.WriteString(fmt.Sprintf("s2=%d\n", s2))
	}
	if h1 != "" {
		uapiBuilder.WriteString(fmt.Sprintf("h1=%s\n", h1))
	}
	if h2 != "" {
		uapiBuilder.WriteString(fmt.Sprintf("h2=%s\n", h2))
	}
	if h3 != "" {
		uapiBuilder.WriteString(fmt.Sprintf("h3=%s\n", h3))
	}
	if h4 != "" {
		uapiBuilder.WriteString(fmt.Sprintf("h4=%s\n", h4))
	}

	// Peers Config
	for _, section := range cfg.Sections() {
		if section.Name() == "Peer" {
			pubKey := section.Key("PublicKey").String()
			presharedKey := section.Key("PresharedKey").String()
			endpoint := section.Key("Endpoint").String()
			keepalive := section.Key("PersistentKeepalive").MustInt(0)
			allowedIPs := section.Key("AllowedIPs").Strings(",")

			uapiBuilder.WriteString(fmt.Sprintf("public_key=%s\n", toHex(pubKey)))
			if presharedKey != "" {
				uapiBuilder.WriteString(fmt.Sprintf("preshared_key=%s\n", toHex(presharedKey)))
			}
			if endpoint != "" {
				uapiBuilder.WriteString(fmt.Sprintf("endpoint=%s\n", endpoint))
			}
			if keepalive != 0 {
				uapiBuilder.WriteString(fmt.Sprintf("persistent_keepalive_interval=%d\n", keepalive))
			}
			for _, ip := range allowedIPs {
				uapiBuilder.WriteString(fmt.Sprintf("allowed_ip=%s\n", strings.TrimSpace(ip)))
			}
		}
	}

	// 6. Apply UAPI Config
	log.Println("Applying configuration via UAPI...")
	err = dev.IpcSet(uapiBuilder.String())
	if err != nil {
		log.Fatalf("Failed to apply configuration: %v", err)
	}

	// 7. Configure System Networking using Netlink (Native Go)
	// This avoids "Operation not permitted" errors with child processes needing capabilities.

	// Get Link
	link, err := netlink.LinkByName(interfaceName)
	if err != nil {
		log.Fatalf("Failed to get link %s: %v", interfaceName, err)
	}

	// Set MTU
	if err := netlink.LinkSetMTU(link, mtu); err != nil {
		log.Fatalf("Failed to set MTU: %v", err)
	}

	// Add Addresses
	for _, addr := range addresses {
		addr = strings.TrimSpace(addr)

		// Parse CIDR
		ipNet, err := netlink.ParseIPNet(addr)
		if err != nil {
			log.Fatalf("Invalid address format %s: %v", addr, err)
		}

		log.Printf("Adding address: %s", addr)
		if err := netlink.AddrAdd(link, &netlink.Addr{IPNet: ipNet}); err != nil {
			log.Fatalf("Failed to add address %s: %v", addr, err)
		}
	}

	// Bring Interface Up
	log.Println("Bringing interface up...")
	if err := netlink.LinkSetUp(link); err != nil {
		log.Fatalf("Failed to set link up: %v", err)
	}

	log.Println("AmneziaWG interface started successfully.")

	// 8. Wait for Signal
	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM)
	signal.Notify(term, os.Interrupt)

	select {
	case <-term:
		log.Println("Received signal, shutting down...")
	case <-dev.Wait():
		log.Println("Device closed unexpectedly")
	}

	// Cleanup is handled by defer statements
	log.Println("Goodbye.")
}
