package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func main() {
	dataFlag := flag.String("data", "", "catalog data directory (defaults to the adjacent 2004 directory)")
	addressFlag := flag.String("addr", "127.0.0.1:0", "listen address (loopback only unless explicitly allowed)")
	allowNonLoopback := flag.Bool("allow-non-loopback", false, "allow binding beyond loopback; intended for isolated containers published only on host loopback")
	noBrowser := flag.Bool("no-browser", false, "print the local URL without opening a browser")
	flag.Parse()

	dataDirectory := *dataFlag
	if dataDirectory == "" {
		var err error
		dataDirectory, err = defaultDataDirectory()
		if err != nil {
			fatal(err)
		}
	}
	catalog, err := loadCatalog(dataDirectory)
	if err != nil {
		fatal(err)
	}
	if err := validateListenAddress(*addressFlag, *allowNonLoopback); err != nil {
		fatal(err)
	}
	listener, err := net.Listen("tcp", *addressFlag)
	if err != nil {
		fatal(fmt.Errorf("listen on local address: %w", err))
	}
	url := "http://" + listener.Addr().String() + "/"
	if tcpAddress, ok := listener.Addr().(*net.TCPAddr); ok && tcpAddress.IP.IsUnspecified() {
		url = "http://" + net.JoinHostPort("127.0.0.1", fmt.Sprint(tcpAddress.Port)) + "/"
	}
	fmt.Printf("PhotoLine Reader: %s\n", catalogSummary(catalog))
	fmt.Printf("Local viewer: %s\n", url)
	if !*noBrowser {
		openBrowser(url)
	}
	server := &http.Server{
		Handler:           newServer(catalog).handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		fatal(fmt.Errorf("serve local viewer: %w", err))
	}
}

func defaultDataDirectory() (string, error) {
	if executable, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(executable), "2004")
		if isCatalogDirectory(candidate) {
			return candidate, nil
		}
	}
	if isCatalogDirectory("2004") {
		return "2004", nil
	}
	if isCatalogDirectory(".") {
		return ".", nil
	}
	return "", fmt.Errorf("could not find TextData.LST; place the 2004 data folder next to the application or pass -data")
}

func isCatalogDirectory(directory string) bool {
	info, err := os.Stat(filepath.Join(directory, "TextData.LST"))
	return err == nil && info.Mode().IsRegular()
}

func validateListenAddress(address string, allowNonLoopback bool) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return nil
	}
	if allowNonLoopback {
		return nil
	}
	return fmt.Errorf("viewer only permits loopback addresses by default; use -allow-non-loopback only in a suitably isolated environment")
}

func catalogSummary(catalog *Catalog) string {
	return fmt.Sprintf("%d counties, %d schools, %d classes, %d people; %d class photos available",
		len(catalog.Counties), len(catalog.Schools), len(catalog.Classes), catalog.PeopleCount, catalog.PhotoMatches)
}

func openBrowser(url string) {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	if err := command.Start(); err != nil {
		fmt.Printf("Could not open a browser automatically; open the local URL above.\n")
		return
	}
	go func() { _ = command.Wait() }()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "PhotoLine Reader:", err)
	os.Exit(1)
}
