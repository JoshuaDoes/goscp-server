package main

import (
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

var (
	srvHost = "0.0.0.0"
	srvPort = "100"

	joinMsg = "This server is running an experimental Go implementation of the kSCP server specification."
	motdMsg = "Welcome to GoSCP!"

	clients = make([]*Client, 0)
)

// Client stores data about a client
type Client struct {
	Socket     *net.TCPConn
	Username   string
	UserAgent  string
	SignOn     time.Time
	LastActive time.Time
	LocalAddr  string
	RemoteAddr string

	/* USER FLAGS:
	   This section covers various user "flags". Flags are used as a permission system,
	   some flags give administration access to users, other take away access to certain things.
	   Current flags:
	   O - This flag is the default flag, it can be found on all online users.
	   M - This flag will make the server ignore all MSG packets sent by the flagged user.
	   A - General server administrator. Users with this flag can change the MOTD or kick/mute users.
	   R - Root-level access flag, usually combined with flag A.
	       Flag R gives a user low-level access (like ban or server shutdown) and makes user invulnerable.
	       (Can't be kicked, banned, or muted)
	   G - Ghost mode, any user with this flag will not show up in LIST requests, or when
	       joining or leaving the server. Useful for bots who should not be seen. (Service bots)
	   B - This flag will set the user as away.
	*/
	Online bool
	Mute   bool
	Admin  bool
	Root   bool
	Ghost  bool
	Away   bool

	//Additional AFK data
	AwayReason   string
	AwayAnnounce bool
}

func main() {
	listenAddr, err := net.ResolveTCPAddr("tcp", srvHost+":"+srvPort)
	if err != nil {
		log.Fatalf("Error resolving bind address [%s:%s]: %v", srvHost, srvPort, err)
	}

	server, err := net.ListenTCP("tcp", listenAddr)
	if err != nil {
		log.Fatalf("Error starting TCP server on [%s:%s]: %v", srvHost, srvPort, err)
	}
	defer server.Close()

	log.Println("Listening on [" + srvHost + ":" + srvPort + "]")

	for {
		conn, err := server.AcceptTCP()
		if err != nil {
			log.Printf("Error accepting connection: %v", err)
		}

		go handleClient(conn)
	}
}

func handleClient(c *net.TCPConn) {
	c.SetKeepAlive(true)
	c.SetKeepAlivePeriod(30 * time.Second)
	c.SetNoDelay(true)

	client := &Client{
		Username:   "Unknown",
		UserAgent:  "Unknown",
		Socket:     c,
		SignOn:     time.Now(),
		LastActive: time.Now(),
		LocalAddr:  c.LocalAddr().String(),
		RemoteAddr: c.RemoteAddr().String(),
	}
	clients = append(clients, client)

	log.Printf("New connection from [%s]", client.RemoteAddr)

	for {
		buf := make([]byte, 65535)

		l, err := c.Read(buf)
		if err != nil {
			log.Printf("Lost connection from [%s]", client.RemoteAddr)
			kickClient(client)
			if client.Username != "Unknown" {
				broadcastMsg("Announcement", client.Username, client.Username+" was disconnected from the server!")
			}
			return
		}
		if l <= 0 {
			continue
		}

		packets := strings.Split(string(buf[:l]), "\r\n")
		for i := 0; i < len(packets); i++ {
			if packets[i] == "" {
				continue
			}

			packet := strings.Split(packets[i], "\x01")
			if len(packet) <= 0 {
				continue
			}

			log.Printf("New packet from [%s]: %v", client.RemoteAddr, packet)

			cmd := packet[0]
			args := make([]string, 0)
			if len(packet) >= 2 {
				args = packet[1:]
			}

			switch cmd {
			case "PING":
				if len(args) < 1 {
					continue
				}
				client.Socket.Write([]byte("PONG\x01" + args[0] + "\r\n"))
			case "JOIN":
				if client.Online {
					continue
				}
				if len(args) < 1 {
					continue
				}

				if len(args) < 2 {
					client.Username = args[0]
				}
				if len(args) < 3 {
					client.Username = args[0]
					client.UserAgent = args[1]
				}

				for _, otherClient := range clients {
					if client.Username == otherClient.Username && client.RemoteAddr != otherClient.RemoteAddr {
						killClient(client, "Username is already in use.")
						return
					}
				}

				client.Online = true

				broadcastMsg("Announcement", client.Username, client.Username+" has joined the chat!")
				sendMsg("Announcement", client.Username, joinMsg)
				sendMsg("Message of the Day", client.Username, motdMsg)
			case "QUIT":
				kickClient(client)
				broadcastMsg("Announcement", client.Username, client.Username+" has left the chat!")
			case "MSG":
				if !client.Online {
					continue
				}
				if len(args) < 2 {
					continue
				}

				broadcastMsg(args[0], args[0], args[1])
			case "PM":
				if !client.Online {
					continue
				}
				if len(args) < 2 {
					continue
				}

				sendPM(client.Username, args[0], args[1])
			case "LIST":
				if !client.Online {
					continue
				}

				sendList(client.Socket)
			case "NAME":
				if !client.Online {
					continue
				}
				if len(args) < 2 {
					continue
				}
				if client.Username != args[0] {
					continue
				}
				client.Username = args[1]

				broadcastMsg("Announcement", "", args[0]+" is now known as "+args[1]+".")
				sendList(client.Socket)
			case "AWAY":
				if !client.Online {
					continue
				}

				if client.Away {
					client.Away = false
					sendMsg("Announcement", client.Username, "You are no longer marked as away.")

					if client.AwayAnnounce {
						broadcastMsg("Announcement", client.Username, client.Username+" is no longer marked as away.")
					}

					client.AwayAnnounce = false
					client.AwayReason = ""
				} else {
					client.Away = true

					if len(args) > 0 {
						client.AwayReason = args[0]
						sendMsg("Announcement", client.Username, "You are now marked as away. Reason: "+args[0])

						if len(args) > 1 {
							if args[1] == "1" {
								client.AwayAnnounce = true
								broadcastMsg("Announcement", client.Username, client.Username+" is now marked as away. Reason: "+args[0])
							}
						}
					} else {
						sendMsg("Announcement", client.Username, "You are now marked as away.")
					}
				}

				sendList(client.Socket)
			default:
				sendMsg("Announcement", client.Username, "Server does not support opcode [b]"+cmd+"[/b].")
			}
		}
	}
}

func broadcastMsg(username, fromUsername, msg string) {
	if len(clients) < 1 {
		return
	}

	for _, client := range clients {
		if client.Username == fromUsername {
			continue
		}

		client.Socket.Write([]byte("MSG\x01" + username + "\x01" + msg + "\r\n"))
	}
}

func sendMsg(fromUsername, toUsername, msg string) {
	if len(clients) < 1 {
		return
	}

	for _, client := range clients {
		if client.Username == toUsername {
			client.Socket.Write([]byte("MSG\x01" + fromUsername + "\x01" + msg + "\r\n"))
			break
		}
	}
}

func sendPM(fromUsername, toUsername, msg string) {
	if len(clients) < 1 {
		return
	}

	for _, client := range clients {
		if client.Username == toUsername {
			client.Socket.Write([]byte("PM\x01" + fromUsername + "\x01" + msg + "\r\n"))
			break
		}
	}
}

func kickClient(client *Client) {
	if len(clients) < 1 {
		return
	}

	newClients := make([]*Client, 0)

	for i := 0; i < len(clients); i++ {
		if client.RemoteAddr == clients[i].RemoteAddr {
			continue
		}
		newClients = append(newClients, clients[i])
	}

	clients = newClients

	client.Socket.Close()
}

func killClient(client *Client, reason string) {
	if len(clients) < 1 {
		return
	}

	if reason == "" {
		client.Socket.Write([]byte("KILL\r\n"))
	} else {
		client.Socket.Write([]byte("KILL\x01" + reason + "\r\n"))
	}

	kickClient(client)
}

func sendList(c *net.TCPConn) {
	if len(clients) < 1 {
		return
	}

	users := make([]string, 0)

	for _, client := range clients {
		if client.Ghost {
			continue
		}
		if !client.Online {
			continue
		}

		flags := "O"
		if client.Mute {
			flags += "M"
		}
		if client.Admin {
			flags += "A"
		}
		if client.Root {
			flags += "R"
		}
		if client.Away {
			flags += "B"
		}

		users = append(users, fmt.Sprintf("[%s] %s - %s", flags, client.Username, client.UserAgent))
	}

	c.Write([]byte("LIST\x01" + strings.Join(users, "\x01") + "\r\n"))
}
