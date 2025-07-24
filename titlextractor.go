package main

import (
    "bufio"
    "context"
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

const (
    colorReset   = "\033[0m"
    colorGreen   = "\033[32m"
    colorYellow  = "\033[33m"
    colorOrange  = "\033[33;1m"
    colorMagenta = "\033[35m"
)

type result struct {
    url, title, err string
    responseCode    int
}

func getTitle(body io.ReadCloser) string {
    defer body.Close()
    tokenizer := html.NewTokenizer(body)
    title := "<title> tag missing"
    for {
        tokenType := tokenizer.Next()
        if tokenType == html.ErrorToken {
            if tokenizer.Err() == io.EOF {
                break
            }
            title = tokenizer.Err().Error()
            break
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

func getWebContent(client *http.Client, wg *sync.WaitGroup, urls <-chan string, results chan<- result) {
    defer wg.Done()
    for url := range urls {
        res := result{url: url}

        ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
        req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
        if err != nil {
            res.err = err.Error()
            results <- res
            cancel()
            continue
        }

        response, err := client.Do(req)
        cancel() // Cancel the context whether it failed or succeeded

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
        return colorGreen
    case statusCode >= 300 && statusCode < 400:
        return colorYellow
    case statusCode >= 400 && statusCode < 500:
        return colorOrange
    case statusCode >= 500:
        return colorMagenta
    default:
        return colorReset
    }
}

func main() {
    var concurrent int
    flag.IntVar(&concurrent, "c", 5, "Number of concurrent workers")
    flag.Parse()

    urls := make(chan string, concurrent*2)
    results := make(chan result, concurrent*2)

    scanner := bufio.NewScanner(os.Stdin)

    // Set large buffer for long URLs
    buf := make([]byte, 0, 1024*1024)
    scanner.Buffer(buf, 1024*1024)

    go func() {
        for scanner.Scan() {
            line := strings.TrimSpace(scanner.Text())
            if line != "" {
                urls <- line
            }
        }
        if err := scanner.Err(); err != nil {
            fmt.Fprintf(os.Stderr, "[Scanner Error] %s\n", err)
        }
        close(urls)
    }()

    var wg sync.WaitGroup
    wg.Add(concurrent)

    client := &http.Client{
        Timeout: 15 * time.Second,
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
