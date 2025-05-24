# httplib

`httplib` is a Go library designed to simplify the creation and management of HTTP servers and clients. It provides a clean and extensible API for building robust HTTP-based applications.

## Features

- **Server Management**: Easily configure and manage HTTP servers with support for graceful shutdowns.
- **Client Utilities**: Simplified HTTP client utilities for making requests.
- **Customizable Options**: Configure server behavior with options like timeouts and custom addresses.
- **Graceful Shutdown**: Ensure active connections are handled properly during shutdown.

## Installation

To install the library, use `go get`:

```bash
go get github.com/fpkg/httplib
```

## Usage

### Server Example

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/fpkg/httplib/server"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, World!")
	})

	srv := server.NewServer(mux, server.WithAddr(":8080"))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		fmt.Println("Server error:", err)
	}
}
```

### Client Example

```go
package main

import (
	"fmt"
	"io"
	"net/http"

	"github.com/fpkg/httplib/client"
)

func main() {
	resp, err := client.Get("https://example.com")
	if err != nil {
		fmt.Println("Request error:", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Println(string(body))
}
```

## Testing

Run the tests using:

```bash
go test ./...
```

## Contributing

Contributions are welcome! Please fork the repository and submit a pull request with your changes.

## License

This project is licensed under the MIT License. See the `LICENSE` file for details.
