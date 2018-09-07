package main

import (
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	srvHost = "0.0.0.0"
	srvPort = "100"

	joinMsg     = "This server is running an experimental Go implementation of the kSCP server specification."
	motdMsg     = "Welcome to GoSCP!"
	colorServer = "#d64161"
	colorAuth   = "#ffcc00"
	colorInfo   = "#f6de6c"

	users = make([]*User, 0)
)

// Session stores data about a session
type Session struct {
	//Connection details
	User          *User        //Pointer to the user for easier access
	Authenticated bool         //Whether or not this session is authenticated
	Socket        *net.TCPConn //Connection socket
	SignOn        time.Time    //Signon time
	LastActive    time.Time    //Last active time
	LocalAddr     string       //Local address of socket
	RemoteAddr    string       //Remote address of socket
}

// User stores data about a user
type User struct {
	//User details
	Username     string
	PasswordHash string
	About        string
	Sessions     []Session

	//User flags
	Online bool
	Mute   bool
	Admin  bool
	Root   bool
	Ghost  bool
	Away   bool

	//Pending private messages
	PendingPMs []PrivateMessage

	//AFK data
	AwayReason   string
	AwayAnnounce bool
}

// PrivateMessage stores data about a private message
type PrivateMessage struct {
	User    *User  //Pointer to the sending user
	Message string //The message for the receiving user
}

// Write writes a packet to the session's TCP socket
func (s *Session) Write(parameters ...string) {
	s.Socket.Write([]byte(strings.Join(parameters, "\x01") + "\r\n"))
}

// BroadcastMsg broadcasts a message from the client to all connected clients
func (s *Session) BroadcastMsg(msg string) {
	for _, user := range users {
		for _, session := range user.Sessions {
			if s.RemoteAddr == session.RemoteAddr {
				continue
			}
			session.Write("MSG", s.User.Username, msg)
		}
	}
}

// BroadcastAction broadcasts an action triggered by the client to all connected clients
func (s *Session) BroadcastAction(msg string) {
	for _, user := range users {
		for _, session := range user.Sessions {
			if s.RemoteAddr == session.RemoteAddr {
				continue
			}
			session.Write("MSG", "Announcement", msg)
		}
	}
}

// SendMsg sends a message from the sending username to the client
func (s *Session) SendMsg(fromUsername, msg string) {
	s.Write("MSG", fromUsername, msg)
}

// SendAuthMsg sends an authentication message to the client
func (s *Session) SendAuthMsg(msg string) {
	s.Write("MSG", "[Authentication]", "[color="+colorAuth+"]"+msg+"[/color]")
}

// SendServerMsg sends a server message to the client
func (s *Session) SendServerMsg(msg string) {
	s.Write("MSG", "[Server]", "[color="+colorServer+"]"+msg+"[/color]")
}

// SendInfoMsg sends an info message to the client
func (s *Session) SendInfoMsg(msg string) {
	s.Write("MSG", "[Info]", "[color="+colorInfo+"]"+msg+"[/color]")
}

// SendAnnouncementMsg sends an announcement message to the client
func (s *Session) SendAnnouncementMsg(msg string) {
	s.Write("Msg", "Announcement", msg)
}

// SendPM sends a private message from the sending username to the client
func (s *Session) SendPM(targetUsername, msg string) {
	for _, user := range users {
		if targetUsername == user.Username {
			if len(user.Sessions) <= 0 {
				user.PendingPMs = append(user.PendingPMs, PrivateMessage{User: s.User, Message: msg})
				break
			}

			for _, session := range user.Sessions {
				session.Write("PM", s.User.Username, msg)
			}
			break
		}
	}
}

// SendPMFrom sends a private message from the sending username to the client
func (s *Session) SendPMFrom(fromusername, msg string) {
	s.Write("PM", fromusername, msg)
}

// SendRAW sends a raw packet from the sending client to the target username
func (s *Session) SendRAW(targetUsername, raw string) {
	for _, user := range users {
		if targetUsername == user.Username {
			for _, session := range user.Sessions {
				if s.RemoteAddr == session.RemoteAddr {
					continue
				}
				session.Write("RAW", targetUsername, raw)
			}
			break
		}
	}
}

// Close closes the connection to the client and removes the client from the clients list
func (s *Session) Close() {
	for _, user := range users {
		if s.User.Username == user.Username {
			newSessions := make([]Session, 0)
			for _, session := range user.Sessions {
				if s.RemoteAddr == session.RemoteAddr {
					continue
				}
				newSessions = append(newSessions, session)
			}
			s.User.Sessions = newSessions
			s.Socket.Close()

			if len(s.User.Sessions) <= 0 {
				s.User.Away = true
				s.User.AwayReason = "Offline."
			}
			break
		}
	}
}

// Kill sends a kill message to the client and then closes the connection
func (s *Session) Kill(reason string) {
	if reason == "" {
		s.Write("KILL")
	} else {
		s.Socket.Write([]byte("KILL\x01" + reason + "\r\n"))
		s.Write("KILL", reason)
	}
	s.Close()
}

// SendList sends the user list to the client
func (s *Session) SendList() {
	userList := make([]string, 0)

	for _, user := range users {
		if user.Ghost {
			continue
		}
		if !user.Online {
			continue
		}

		flags := "O"
		if user.Mute {
			flags += "M"
		}
		if user.Admin {
			flags += "A"
		}
		if user.Root {
			flags += "R"
		}
		if user.Away {
			flags += "B"
		}

		userList = append(userList, fmt.Sprintf("[%s] %s - %s", flags, user.Username, user.About))
	}

	s.Write("LIST", strings.Join(userList, "\x01"))
}

// Ping sends a ping to the client
func (s *Session) Ping(msg string) {
	s.Write("PING", msg)
}

// Pong sends a pong to the client
func (s *Session) Pong(msg string) {
	s.Write("PONG", msg)
}

func handleClient(c *net.TCPConn) {
	c.SetKeepAlive(true)
	c.SetKeepAlivePeriod(30 * time.Second)
	c.SetNoDelay(true)

	session := Session{
		Socket:     c,
		SignOn:     time.Now(),
		LastActive: time.Now(),
		LocalAddr:  c.LocalAddr().String(),
		RemoteAddr: c.RemoteAddr().String(),
	}

	log.Printf("New connection from [%s]", session.RemoteAddr)

	for {
		buf := make([]byte, 65535)

		l, err := c.Read(buf)
		if err != nil {
			log.Printf("Lost connection from [%s]", session.RemoteAddr)
			session.Close()
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

			log.Printf("New packet from [%s]: %v", session.RemoteAddr, strings.Join(packet, "|"))

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
				session.Pong(args[0])
			case "PONG": //Temporary NOP opcode until routine ping checks are implemented
				continue
			case "JOIN":
				if session.User != nil {
					if session.User.Online {
						continue
					}
				}
				if len(args) < 1 {
					continue
				}

				user := User{
					Username: args[0],
				}
				if len(args) > 1 {
					user.About = args[1]
				}
				session.User = &user

				session.Ping(string(time.Now().Unix()))

				if userExists(args[0]) {
					session.User = getUser(args[0])
					session.SendAuthMsg("You must authenticate with the password attached to your username to continue. Ex: [u]/auth MyPassword[/u]")
				} else {
					session.SendAuthMsg("You must register a password to attach to your username to continue. Ex: [u]/auth MyNewPassword[/u]")
				}
			case "AUTH":
				if len(args) < 1 {
					continue
				}

				password := []byte(args[0])

				if session.Authenticated {
					hashedPassword, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
					if err != nil {
						session.SendAuthMsg("There was an error changing your password. Please try again.")
						continue
					}
					session.User.PasswordHash = string(hashedPassword)
					session.SendAuthMsg("Changed your password successfully.")
					continue
				}

				if session.User.PasswordHash != "" {
					err := bcrypt.CompareHashAndPassword([]byte(session.User.PasswordHash), password)
					if err != nil {
						session.SendAuthMsg("You entered an invalid password. Please try again.")
						continue
					}
					session.SendAuthMsg("Authenticated as " + session.User.Username + " successfully.")
				} else {
					if userExists(session.User.Username) {
						session.SendAuthMsg("The username " + session.User.Username + " is already taken. Please change your username and try again. Ex: [u]/nick MyNewUsername[/u]")
						continue
					}

					hashedPassword, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
					if err != nil {
						session.SendAuthMsg("There was an error registering your username with your chosen password. Please try again.")
						continue
					}
					session.User.PasswordHash = string(hashedPassword)
					users = append(users, session.User)
					session.SendAuthMsg("Registered your username with your chosen password successfully.")
					session.BroadcastAction(session.User.Username + " has joined the server!")
				}

				session.User.Sessions = append(session.User.Sessions, session)
				session.User.Online = true
				session.User.Away = false
				session.User.AwayReason = ""
				session.Authenticated = true

				session.SendList()
				session.SendAnnouncementMsg(joinMsg)
				session.SendMsg("Message of the Day", motdMsg)

				for _, pm := range session.User.PendingPMs {
					session.SendPMFrom(pm.User.Username, pm.Message)
				}
			case "QUIT":
				session.Close()
				break
			case "MSG":
				if !session.Authenticated {
					continue
				}
				if len(args) < 2 {
					continue
				}
				if args[0] != session.User.Username {
					continue
				}

				session.BroadcastMsg(args[1])
			case "PM":
				if !session.Authenticated {
					continue
				}
				if len(args) < 2 {
					continue
				}

				if !userExists(args[0]) {
					session.SendServerMsg("The user " + args[0] + " does not exist.")
					continue
				}

				session.SendPM(args[0], args[1])
			case "LIST":
				if !session.Authenticated {
					continue
				}

				session.SendList()
			case "NAME":
				if len(args) < 2 {
					continue
				}
				if session.User.Username != args[0] {
					continue
				}

				if userExists(args[1]) {
					if session.Authenticated {
						session.SendServerMsg("The username " + args[0] + " is already taken.")
					} else {
						session.SendAuthMsg("The username " + args[0] + " is already taken.")
					}
					continue
				}

				session.User.Username = args[1]

				if session.Authenticated {
					session.BroadcastAction(args[0] + " is now known as " + args[1] + ".")
					session.SendList()
				} else {
					session.SendAnnouncementMsg(args[0] + " is now known as " + args[1] + ".")
				}
			case "INFO":
				if !session.User.Online {
					continue
				}
				if len(args) < 1 {
					continue
				}

				if !userExists(args[0]) {
					session.SendServerMsg("The user " + args[0] + " does not exist.")
					continue
				}

				user := getUser(args[0])
				userInfo := fmt.Sprintf("Info for user %s:%%n%%Online: %v%%n%%Away: %v%%n%%", user.Username, user.Online, user.Away)
				if user.AwayReason != "" {
					userInfo += fmt.Sprintf("Away reason: %s%%n%%", user.AwayReason)
				}
				userInfo += fmt.Sprintf("Muted: %v%%n%%Admin: %v%%n%%Root: %v%%n%%", user.Mute, user.Admin, user.Root)

				session.SendInfoMsg(strings.TrimPrefix(userInfo, "%%n%%"))
			case "RAW":
				if !session.User.Online {
					continue
				}
				if len(args) < 2 {
					continue
				}

				if !userExists(args[0]) {
					session.SendServerMsg("The user " + args[0] + " does not exist.")
					continue
				}

				session.SendRAW(args[0], args[1])
			case "AWAY":
				if !session.User.Online {
					continue
				}

				if session.User.Away {
					session.User.Away = false
					session.SendInfoMsg("You are no longer marked as away.")

					if session.User.AwayAnnounce {
						session.BroadcastAction(session.User.Username + " is no longer marked as away.")
					}

					session.User.AwayAnnounce = false
					session.User.AwayReason = ""
				} else {
					session.User.Away = true

					if len(args) > 0 {
						session.User.AwayReason = args[0]
						session.SendInfoMsg("You are now marked as away. Reason: " + args[0])

						if len(args) > 1 {
							if args[1] == "1" {
								session.User.AwayAnnounce = true
								session.BroadcastAction(session.User.Username + " is now marked as away. Reason: " + args[0])
							}
						}
					} else {
						session.SendInfoMsg("You are now marked as away.")
					}
				}

				session.SendList()
			default:
				session.SendServerMsg("Server does not support opcode [b]" + cmd + "[/b].")
			}
		}
	}
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

func userExists(username string) bool {
	for _, user := range users {
		if username == user.Username && user.PasswordHash != "" && user.Ghost == false {
			return true
		}
	}
	return false
}
func getUser(username string) *User {
	for _, user := range users {
		if username == user.Username && user.PasswordHash != "" && user.Ghost == false {
			return user
		}
	}
	return nil
}
