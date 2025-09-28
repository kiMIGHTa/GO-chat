# GO-chat

GO-chat is a chat application built using the Go programming language. This project demonstrates a simple yet effective way to implement real-time messaging with Go, focusing on concurrency, networking, and a clean code structure. Whether you're learning Go, exploring web sockets, or want a foundation for a chat service, GO-chat is a great starting point.

## Features

- Real-time messaging between multiple users
- WebSocket-based communication for low-latency updates
- Simple and intuitive web interface
- User-friendly design for quick testing and demos
- Written in idiomatic, clean Go code

## Getting Started

### Prerequisites

- [Go](https://golang.org/dl/) 1.17 or newer installed on your machine

### Installation

1. **Clone the repository:**
   ```bash
   git clone https://github.com/kiMIGHTa/GO-chat.git
   cd GO-chat
   ```

2. **Install dependencies:**
   Most dependencies are handled via Go modules. If you see a `go.mod` file, simply run:
   ```bash
   go mod tidy
   ```

3. **Run the application:**
   ```bash
   go run main.go
   ```
   Or, if you want to build the binary:
   ```bash
   go build -o go-chat
   ./go-chat
   ```

4. **Access the chat:**
   Open your browser and go to [http://localhost:8080](http://localhost:8080).

## Usage

- Open the chat application in your browser.
- Enter a nickname and start chatting in real-time with other users connected to the server.
- Open multiple browser windows/tabs to simulate multiple users.

## Project Structure

```
go-chat/
├── main.go
├── handlers.go
├── hub.go
├── client.go
├── static/
│   └── ...
└── templates/
    └── ...
```

- `main.go`: Application entry point, sets up routes and server
- `handlers.go`: HTTP and WebSocket handler functions
- `hub.go`: Manages the chat room and message broadcasting
- `client.go`: Represents connected chat clients
- `static/`: Static assets (CSS, JS)
- `templates/`: HTML templates

## Contributing

Contributions are welcome! If you want to add features, fix bugs, or improve documentation, feel free to open an issue or submit a pull request.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Inspired by the official [Go WebSocket Chat example](https://github.com/gorilla/websocket/tree/master/examples/chat)
- Thanks to the Go community for awesome libraries and resources

---
