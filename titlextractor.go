package main

import (
    "bufio"
    "crypto/tls"
    "flag"
    "fmt"
    "io"
    "net"
    "net/http"
    "os"
    "strings"
    "sync"
    "time"

    "golang.org/x/net/html"
)

// ANSI color codes
const (
    colorReset  = "\033[0m"
    colorGreen  = "\033[32m"
    colorYellow = "\033[33m"  // Yellow (for 3xx)
    colorOrange = "\033[33;1m" // Brighter yellow as "orange" (for 4xx)
    colorMagenta = "\033[35m"  // Magenta (for 5xx)
)

type result struct {
    url, title, err string
    responseCode    int
}

// Extract the title from the HTML body
func getTitle(body io.ReadCloser) string {
    defer body.Close()
    tokenizer := html.NewTokenizer(body)
    title := "<title> tag missing"
    for {
        tokenType := tokenizer.Next()
        if tokenType == html.ErrorToken {
            if err := tokenizer.Err(); err == io.EOF {
                break
            } else {
                title = err.Error()
            }
        }
        if tokenType == html.StartTagToken {
            token := tokenizer.Token()
            if token.Data == "title" {
                _ = tokenizer.Next()
                title = tokenizer.Token().Data
                break
            }
        }
    }
    return strings.TrimSpace(strings.Join(strings.Fields(title), " "))
}

// Fetches content for a given URL and extracts its title
func getWebContent(client *http.Client, wg *sync.WaitGroup, urls <-chan string, results chan<- result) {
    defer wg.Done()
    for url := range urls {
        res := result{url: url}
        response, err := client.Get(url)
        if err != nil {
            res.err = err.Error()
            results <- res
            continue
        }

        res.responseCode = response.StatusCode
        res.title = getTitle(response.Body)
        results <- res
    }
}

func colorForStatusCode(statusCode int) string {
    switch {
    case statusCode >= 200 && statusCode < 300:
        return colorGreen  // 2xx: Success
    case statusCode >= 300 && statusCode < 400:
        return colorYellow // 3xx: Redirection
    case statusCode >= 400 && statusCode < 500:
        return colorOrange // 4xx: Client Error (as "orange")
    case statusCode >= 500:
        return colorMagenta // 5xx: Server Error
    default:
        return colorReset   // No color for unhandled codes
    }
}

func main() {
    var concurrent int
    var forceFlag bool

    flag.IntVar(&concurrent, "c", 5, "Number of concurrent workers")
    flag.BoolVar(&forceFlag, "f", false, "Force flag (currently not used)")
    flag.Parse()

    // Reading URLs from stdin in a piped manner
    scanner := bufio.NewScanner(os.Stdin)
    urls := make(chan string)
    go func() {
        for scanner.Scan() {
            urls <- scanner.Text()
        }
        close(urls)
    }()

    results := make(chan result)
    var wg sync.WaitGroup
    wg.Add(concurrent)

    // Setting up HTTP client with proper timeout
    client := &http.Client{
        Timeout: 10 * time.Second,
        Transport: &http.Transport{
            DialContext: (&net.Dialer{
                Timeout:   5 * time.Second,
                KeepAlive: 30 * time.Second,
            }).DialContext,
            TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
        },
    }

    for i := 0; i < concurrent; i++ {
        go getWebContent(client, &wg, urls, results)
    }

    go func() {
        wg.Wait()
        close(results)
    }()

    for res := range results {
        color := colorForStatusCode(res.responseCode)
        if res.err != "" {
            fmt.Printf("%s[Error] %s: %s%s\n", colorMagenta, res.url, res.err, colorReset)
        } else {
            fmt.Printf("%s[%d] %s: %s%s\n", color, res.responseCode, res.url, res.title, colorReset)
        }
    }
}
