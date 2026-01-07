package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"
)

type advertisedType struct {
	ST  string
	USN string
}

const (
	ssdpAddr          = "239.255.255.250:1900"
	serverField       = "Linux/3.10.0 UPnP/1.0 DLNADOC/1.50 GoStream/1.0"
	configID          = 1
	ssdpNotifyDelay   = 50 * time.Millisecond
	ssdpResponseDelay = 10 * time.Millisecond
)

var bootID = time.Now().UTC().Unix()

func getAdvertisedTypes(deviceUUID string) []advertisedType {
	// All types that should be advertised per DLNA spec
	return []advertisedType{
		// Root device
		{
			ST:  "upnp:rootdevice",
			USN: deviceUUID + "::upnp:rootdevice",
		},
		// deviceUUID
		{
			ST:  deviceUUID,
			USN: deviceUUID,
		},
		// Device type
		{
			ST:  "urn:schemas-upnp-org:device:MediaServer:1",
			USN: deviceUUID + "::urn:schemas-upnp-org:device:MediaServer:1",
		},
		// ContentDirectory service
		{
			ST:  "urn:schemas-upnp-org:service:ContentDirectory:1",
			USN: deviceUUID + "::urn:schemas-upnp-org:service:ContentDirectory:1",
		},
		// ConnectionManager service
		{
			ST:  "urn:schemas-upnp-org:service:ConnectionManager:1",
			USN: deviceUUID + "::urn:schemas-upnp-org:service:ConnectionManager:1",
		},
	}
}

func StartSSDP(ctx context.Context, logger *slog.Logger, hostIP string, port int, deviceUUID string) {
	addr, err := net.ResolveUDPAddr("udp", ssdpAddr)
	if err != nil {
		logger.Error("SSDP resolve", "error", err)
		return
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		logger.Error("SSDP dial", "error", err)
		return
	}

	targets := getAdvertisedTypes(deviceUUID)

	go func() {
		defer conn.Close()

		sendSSDPNotify(conn, logger, hostIP, port, targets)

		tickInterval := 30 * time.Second
		ticker := time.NewTicker(tickInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logger.Info("stopping SSDP broadcaster")
				sendSSDPByebye(conn, targets)
				return
			case <-ticker.C:
				sendSSDPNotify(conn, logger, hostIP, port, targets)
			}
		}
	}()
}

func sendSSDPNotify(conn *net.UDPConn, logger *slog.Logger, hostIP string, port int, targets []advertisedType) {
	logger.Debug("broadcasting SSDP notify", "num_types", len(targets))

	for _, t := range targets {
		msg := fmt.Sprintf(
			"NOTIFY * HTTP/1.1\r\n"+
				"HOST: %s\r\n"+
				"CACHE-CONTROL: max-age=1800\r\n"+
				"LOCATION: http://%s:%d/description.xml\r\n"+
				"NT: %s\r\n"+
				"NTS: ssdp:alive\r\n"+
				"SERVER: %s\r\n"+
				"USN: %s\r\n"+
				"BOOTID.UPNP.ORG: %d\r\n"+
				"CONFIGID.UPNP.ORG: %d\r\n"+
				"\r\n",
			ssdpAddr, hostIP, port, t.ST, serverField, t.USN, bootID, configID,
		)

		if _, err := conn.Write([]byte(msg)); err != nil {
			logger.Error("SSDP write", "error", err)
		}
		time.Sleep(ssdpNotifyDelay)
	}
}

func sendSSDPByebye(conn *net.UDPConn, targets []advertisedType) {
	for _, t := range targets {
		msg := fmt.Sprintf(
			"NOTIFY * HTTP/1.1\r\n"+
				"HOST: %s\r\n"+
				"NT: %s\r\n"+
				"NTS: ssdp:byebye\r\n"+
				"USN: %s\r\n"+
				"BOOTID.UPNP.ORG: %d\r\n"+
				"\r\n",
			ssdpAddr, t.ST, t.USN, bootID,
		)
		conn.Write([]byte(msg))
		time.Sleep(ssdpNotifyDelay) // 50ms is enough to prevent bursts
	}
}

func ListenForSearch(ctx context.Context, logger *slog.Logger, hostIP string, port int, deviceUUID string) {
	addr, err := net.ResolveUDPAddr("udp", ssdpAddr)
	if err != nil {
		logger.Error("resolve UDP address", "error", err)
		return
	}

	conn, err := net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		logger.Error("M-SEARCH listener", "error", err)
		return
	}

	go func() {
		<-ctx.Done()
		logger.Info("stopping M-SEARCH listener")
		conn.Close()
	}()

	targets := getAdvertisedTypes(deviceUUID)

	go func() {
		defer conn.Close()
		buf := make([]byte, 2048)

		for {
			n, src, err := conn.ReadFromUDP(buf)
			if err != nil {
				// check if a shutdown is in progress
				if ctx.Err() != nil {
					return
				}

				logger.Error("UDP read error", "error", err)
				return
			}

			msg := string(buf[:n])
			if strings.Contains(msg, "M-SEARCH") {
				logger.Debug("received M-SEARCH", "source", src)

				// Extract ST (Search Target) if present
				searchTarget := "ssdp:all"
				lines := strings.Split(msg, "\r\n")
				for _, line := range lines {
					if strings.HasPrefix(strings.ToUpper(line), "ST:") {
						parts := strings.SplitN(line, ":", 2)
						if len(parts) == 2 {
							searchTarget = strings.TrimSpace(parts[1])
						}
						break
					}
				}

				RespondToSearch(logger, src, hostIP, port, searchTarget, targets)
			}
		}
	}()
}

func RespondToSearch(logger *slog.Logger, dst *net.UDPAddr, hostIP string, port int, searchTarget string, targets []advertisedType) {
	conn, err := net.DialUDP("udp", nil, dst)
	if err != nil {
		logger.Error("respond to search: could not dial udp", "error", err)
		return
	}
	defer conn.Close()

	// Respond with matching types
	for _, t := range targets {
		// Check if this type matches the search target
		if searchTarget != "ssdp:all" && searchTarget != t.ST {
			continue
		}

		response := fmt.Sprintf(
			"HTTP/1.1 200 OK\r\n"+
				"CACHE-CONTROL: max-age=1800\r\n"+
				"DATE: %s\r\n"+
				"EXT:\r\n"+
				"LOCATION: http://%s:%d/description.xml\r\n"+
				"SERVER: %s\r\n"+
				"ST: %s\r\n"+
				"USN: %s\r\n"+
				"BOOTID.UPNP.ORG: %d\r\n"+
				"CONFIGID.UPNP.ORG: %d\r\n"+
				"\r\n",
			time.Now().UTC().Format(time.RFC1123),
			hostIP, port, serverField, t.ST, t.USN, bootID, configID,
		)

		if _, err := conn.Write([]byte(response)); err != nil {
			logger.Error("write response to search", "error", err)
		}
		time.Sleep(ssdpResponseDelay)
	}
}
