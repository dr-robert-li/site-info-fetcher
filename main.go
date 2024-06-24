package main

import (
	"bufio"
	"crypto/tls"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// SiteInfo holds the information about the WordPress site
type SiteInfo struct {
	URL              string
	PHPVersion       string
	MySQLVersion     string
	WordPressVersion string
	Caching          bool
	CacheControl     string
	WebServer        string
	WebServerVersion string
	SSLValid         string
	TTFBs            []time.Duration
	AverageTTFB      time.Duration
	XPoweredBy       string
	PHPStatus        string
	MySQLStatus      string
	WebServerStatus  string
	WordPressStatus  string
}

// fetchURL fetches the URL and returns the response along with the TTFB
func fetchURL(url string) (*http.Response, time.Duration, error) {
	// Ensure the URL includes a protocol scheme
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "http://" + url
	}

	var ttfb time.Duration
	var resp *http.Response
	var err error

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	for i := 0; i < 5; i++ {
		start := time.Now()

		trace := &httptrace.ClientTrace{
			GotFirstResponseByte: func() {
				ttfb = time.Since(start)
			},
		}

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, 0, err
		}
		req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

		resp, err = client.Do(req)
		if err == nil {
			return resp, ttfb, nil
		}

		if !strings.Contains(err.Error(), "Client.Timeout exceeded while awaiting headers") {
			return nil, 0, err
		}

		fmt.Printf("Retrying %d/5 for URL: %s\n", i+1, url)
		time.Sleep(2 * time.Second)
	}

	return nil, 0, err
}

// checkSSL checks if the site has a valid SSL certificate
func checkSSL(url string) (bool, error) {
	// Ensure the URL includes a protocol scheme
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}

	// Remove the protocol scheme for tls.Dial
	host := strings.TrimPrefix(url, "https://")
	host = strings.TrimPrefix(host, "http://")

	conn, err := tls.Dial("tcp", host+":443", nil)
	if err != nil {
		if strings.Contains(err.Error(), "certificate is expired") {
			return false, fmt.Errorf("expired")
		}
		return false, err
	}
	defer conn.Close()

	// Check the certificate
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) > 0 {
		cert := certs[0]
		now := time.Now()
		if now.After(cert.NotBefore) && now.Before(cert.NotAfter) {
			return true, nil
		}
	}
	return false, nil
}

// parseHeaders parses the HTTP headers to extract information
func parseHeaders(headers http.Header) (string, string, bool, string, string, string, string) {
	var webServer, webServerVersion string
	var caching bool
	var cacheControl, xPoweredBy, phpVersion string

	for key, values := range headers {
		lowerKey := strings.ToLower(key)
		for _, value := range values {
			if lowerKey == "server" {
				parts := strings.Split(value, "/")
				webServer = parts[0]
				if len(parts) > 1 {
					webServerVersion = parts[1]
				}
			}
			if lowerKey == "x-powered-by" {
				xPoweredBy = value
				if strings.Contains(value, "PHP") {
					parts := strings.Split(value, "/")
					if len(parts) > 1 {
						phpVersion = parts[1]
					}
				}
			}
			if lowerKey == "cache-control" {
				cacheControl = value
				if strings.Contains(value, "max-age=0") {
					caching = false
				} else if strings.Contains(value, "max-age") {
					caching = true
				}
			}
		}
	}
	return phpVersion, "", caching, webServer, webServerVersion, cacheControl, xPoweredBy
}

// parseHTML parses the HTML content to extract the WordPress version
func parseHTML(body string) string {
	re := regexp.MustCompile(`content="WordPress (\d+\.\d+(\.\d+)?)"`)
	matches := re.FindStringSubmatch(body)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// fetchSupportedVersions fetches the supported versions from the endoflife.date API
func fetchSupportedVersions(product string) ([]map[string]interface{}, error) {
	url := fmt.Sprintf("https://endoflife.date/api/%s.json", product)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var versions []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return nil, err
	}

	return versions, nil
}

// isSupported checks if a version is supported
func isSupported(version string, supportedVersions []map[string]interface{}) bool {
	for _, v := range supportedVersions {
		if cycle, ok := v["cycle"].(string); ok && strings.HasPrefix(version, cycle) {
			if eol, ok := v["eol"].(interface{}); ok {
				if eol == false {
					return true
				}
				if eolDate, ok := eol.(string); ok {
					eolTime, err := time.Parse("2006-01-02", eolDate)
					if err == nil && eolTime.After(time.Now()) {
						return true
					}
				}
			}
		}
	}
	return false
}

// getSupportStatus checks if the versions are supported
func getSupportStatus(phpVersion, mysqlVersion, wpVersion, webServer, webServerVersion string) (string, string, string, string) {
	phpStatus := "Unknown"
	mysqlStatus := "Unknown"
	wpStatus := "Unknown"
	webServerStatus := "Unknown"

	phpVersions, err := fetchSupportedVersions("PHP")
	if err == nil && phpVersion != "" {
		if isSupported(phpVersion, phpVersions) {
			phpStatus = "Supported"
		} else {
			phpStatus = "Outdated"
		}
	}

	mysqlVersions, err := fetchSupportedVersions("mysql")
	if err == nil && mysqlVersion != "" {
		if isSupported(mysqlVersion, mysqlVersions) {
			mysqlStatus = "Supported"
		} else {
			mysqlStatus = "Outdated"
		}
	}

	wpVersions, err := fetchSupportedVersions("WordPress")
	if err == nil && wpVersion != "" {
		if isSupported(wpVersion, wpVersions) {
			wpStatus = "Supported"
		} else {
			wpStatus = "Outdated"
		}
	}

	if webServer != "" && webServerVersion != "" {
		webServerVersions, err := fetchSupportedVersions(webServer)
		if err == nil {
			if isSupported(webServerVersion, webServerVersions) {
				webServerStatus = "Supported"
			} else {
				webServerStatus = "Outdated"
			}
		}
	}

	return phpStatus, mysqlStatus, webServerStatus, wpStatus
}

// getSiteInfo gets the site information for a given URL
func getSiteInfo(url string) (*SiteInfo, error) {
	var ttfs []time.Duration
	for i := 0; i < 3; i++ {
		resp, ttfb, err := fetchURL(url)
		if err != nil {
			return nil, fmt.Errorf("error fetching URL %s: %w", url, err)
		}
		if resp == nil {
			return nil, fmt.Errorf("no response for URL %s after retries", url)
		}
		defer resp.Body.Close()
		ttfs = append(ttfs, ttfb)
	}

	// Calculate the average TTFB
	var totalTTFB time.Duration
	for _, ttfb := range ttfs {
		totalTTFB += ttfb
	}
	averageTTFB := totalTTFB / 3

	// Sort TTFBs in order of longest to shortest latency
	sort.Slice(ttfs, func(i, j int) bool {
		return ttfs[i] > ttfs[j]
	})

	// Print TTFB tests and average in the terminal
	fmt.Printf("Fetching site info for URL: %s - TTFB1: %.3fms, TTFB2: %.3fms, TTFB3: %.3fms, Average TTFB: %.3fms\n",
		url, ttfs[0].Seconds()*1000, ttfs[1].Seconds()*1000, ttfs[2].Seconds()*1000, averageTTFB.Seconds()*1000)

	resp, _, err := fetchURL(url)
	if err != nil {
		return nil, fmt.Errorf("error fetching URL %s: %w", url, err)
	}
	if resp == nil {
		return nil, fmt.Errorf("no response for URL %s after retries", url)
	}
	defer resp.Body.Close()

	phpVersion, mysqlVersion, caching, webServer, webServerVersion, cacheControl, xPoweredBy := parseHeaders(resp.Header)

	// Read the body
	buf := new(strings.Builder)
	_, err = io.Copy(buf, resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body for URL %s: %w", url, err)
	}
	body := buf.String()

	wpVersion := parseHTML(body)

	// Check SSL certificate
	sslValid, sslErr := checkSSL(url)
	if sslErr != nil {
		if sslErr.Error() == "expired" {
			return &SiteInfo{
				URL:      url,
				SSLValid: "Expired",
			}, nil
		}
		return nil, sslErr
	}

	// Get support status
	phpStatus, mysqlStatus, webServerStatus, wpStatus := getSupportStatus(phpVersion, mysqlVersion, wpVersion, webServer, webServerVersion)

	return &SiteInfo{
		URL:              url,
		PHPVersion:       phpVersion,
		MySQLVersion:     mysqlVersion,
		WordPressVersion: wpVersion,
		Caching:          caching,
		CacheControl:     cacheControl,
		WebServer:        webServer,
		WebServerVersion: webServerVersion,
		SSLValid:         fmt.Sprintf("%t", sslValid),
		TTFBs:            ttfs,
		AverageTTFB:      averageTTFB,
		XPoweredBy:       xPoweredBy,
		PHPStatus:        phpStatus,
		MySQLStatus:      mysqlStatus,
		WebServerStatus:  webServerStatus,
		WordPressStatus:  wpStatus,
	}, nil
}

// readCSV reads the CSV file and returns the URLs from the specified column
func readCSV(filePath string, column int) ([]string, error) {
	fmt.Printf("Reading CSV file: %s\n", filePath) // Debugging output
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var urls []string
	for _, record := range records {
		if column < len(record) {
			urls = append(urls, record[column])
		}
	}
	return urls, nil
}

// writeCSV writes the site information to a CSV file
func writeCSV(filePath string, siteInfos []*SiteInfo) error {
	fmt.Printf("Writing results to CSV file: %s\n", filePath) // Debugging output
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	writer.Write([]string{"URL", "PHP Version", "MySQL Version", "WordPress Version", "Caching", "Cache Control", "Web Server", "Web Server Version", "SSL Valid", "TTFB1 - Longest (ms)", "TTFB2 (ms)", "TTFB3 - Shortest (ms)", "Average TTFB (ms)", "X-Powered-By", "PHP Status", "MySQL Status", "Web Server Status", "WordPress Status"})

	// Write site information
	for _, info := range siteInfos {
		ttfb1 := ""
		ttfb2 := ""
		ttfb3 := ""
		averageTTFB := ""

		if len(info.TTFBs) > 0 {
			ttfb1 = fmt.Sprintf("%.3f", info.TTFBs[0].Seconds()*1000) // TTFB1 - Longest in ms
		}
		if len(info.TTFBs) > 1 {
			ttfb2 = fmt.Sprintf("%.3f", info.TTFBs[1].Seconds()*1000) // TTFB2 in ms
		}
		if len(info.TTFBs) > 2 {
			ttfb3 = fmt.Sprintf("%.3f", info.TTFBs[2].Seconds()*1000) // TTFB3 - Shortest in ms
		}
		if info.AverageTTFB != 0 {
			averageTTFB = fmt.Sprintf("%.3f", info.AverageTTFB.Seconds()*1000) // Average TTFB in ms
		}

		writer.Write([]string{
			info.URL,
			info.PHPVersion,
			info.MySQLVersion,
			info.WordPressVersion,
			fmt.Sprintf("%t", info.Caching),
			info.CacheControl,
			info.WebServer,
			info.WebServerVersion,
			info.SSLValid,
			ttfb1,
			ttfb2,
			ttfb3,
			averageTTFB,
			info.XPoweredBy,
			info.PHPStatus,
			info.MySQLStatus,
			info.WebServerStatus,
			info.WordPressStatus,
		})
	}
	return nil
}

func main() {
	reader := bufio.NewReader(os.Stdin)

	// Prompt the user for the CSV file path
	fmt.Print("Enter the path to the CSV file: ")
	csvFilePath, _ := reader.ReadString('\n')
	csvFilePath = strings.TrimSpace(csvFilePath)

	// Prompt the user for the column number containing the URLs
	fmt.Print("Enter the column number containing the URLs (starting from 0): ")
	var column int
	fmt.Scanf("%d", &column)

	// Read URLs from the CSV file
	urls, err := readCSV(csvFilePath, column)
	if err != nil {
		fmt.Printf("Error reading CSV file: %v\n", err)
		return
	}

	var siteInfos []*SiteInfo
	for _, url := range urls {
		info, err := getSiteInfo(url)
		if err != nil {
			fmt.Printf("Error fetching site info for %s: %v\n", url, err)
			continue
		}
		siteInfos = append(siteInfos, info)
	}

	// Generate the output file name with timestamp
	timestamp := time.Now().Format("20060102_150405")
	outputFilePath := fmt.Sprintf("site_info_%s.csv", timestamp)

	// Write the results to a CSV file
	err = writeCSV(outputFilePath, siteInfos)
	if err != nil {
		fmt.Printf("Error writing CSV file: %v\n", err)
		return
	}

	fmt.Printf("Site information written to %s\n", outputFilePath)
}
