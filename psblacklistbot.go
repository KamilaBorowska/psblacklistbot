package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/xfix/showdown2irc/showdown"
)

type bannedUser struct {
	User showdown.UserID
	Room showdown.RoomID
}

type banlistMutex struct {
	banlist map[bannedUser]struct{}
	sync.RWMutex
}

var banlist banlistMutex

func initializeBlacklist() error {
	fileHandle, err := os.Open("blacklist.json")
	if err != nil {
		return err
	}
	defer fileHandle.Close()
	decoder := json.NewDecoder(bufio.NewReader(fileHandle))
	var users []bannedUser
	decoder.Decode(&users)
	banlist.banlist = map[bannedUser]struct{}{}
	for _, user := range users {
		banlist.banlist[user] = struct{}{}
	}
	return nil
}

type configurationOptions struct {
	Nickname string
	Password string
}

func chatMessage(message string, room *showdown.Room) {
	parts := strings.SplitN(message, "|", 3)
	nickname := parts[1]
	if nickname != "%xfix" && (nickname[0] == ' ' || nickname[0] == '+' || nickname[0] == '%') {
		checkBlacklist(parts[1], room)
	} else {
		message := parts[2]
		if len(message) >= 1 && message[0] == '.' {
			commandParts := strings.SplitN(message[1:], " ", 2)
			if len(commandParts) >= 1 {
				command := commandParts[0]
				argument := ""
				if len(commandParts) == 2 {
					argument = commandParts[1]
				}
				if callback, ok := chatCommands[command]; ok {
					callback(argument, room)
				}
			}
		}
	}
}

func privateMessage(message string, room *showdown.Room) {
	parts := strings.SplitN(message, "|", 3)
	if parts[0] != " Usain Bot" {
		room.SendCommand("pm", fmt.Sprintf("%s, Hi, I'm a temporary replacement supporting only blacklisting (ab and unab commands) for Usain Bot until blacklists will be implemented on Showdown. For room help, try asking room auth.", parts[0]))
		room.SendCommand("pm", fmt.Sprintf("%s, Old Usain Bot features were moved to Showdown code proper, use /roomsettings as a room owner to manage these.", parts[0]))
	}
}

func checkBlacklist(nickname string, room *showdown.Room) {
	userID := showdown.ToID(nickname)
	banlist.RLock()
	if _, ok := banlist.banlist[bannedUser{User: userID, Room: room.ID}]; ok {
		room.SendCommand("roomban", fmt.Sprintf("%s, Blacklisted", string(userID)))
	}
	banlist.RUnlock()
}

func parseRooms(popup string, room *showdown.Room) {
	for _, message := range strings.Split(popup, "||||") {
		const roomAuthMessage = "Room auth: "
		if strings.HasPrefix(message, roomAuthMessage) {
			for _, roomName := range strings.Split(message[len(roomAuthMessage):], ", ") {
				if roomName[0] == '*' {
					room.SendCommand("join", roomName)
				}
			}
		}
	}
}

var commands = map[string]func(string, *showdown.Room){
	"c:":    chatMessage,
	"j":     checkBlacklist,
	"J":     checkBlacklist,
	"popup": parseRooms,
	"pm":    privateMessage,
}

func autoban(name string, room *showdown.Room) {
	room.Reply("Blacklisted")
	banlist.Lock()
	banlist.banlist[bannedUser{User: showdown.ToID(name), Room: room.ID}] = struct{}{}
	banlist.Unlock()
	checkBlacklist(name, room)
}

func unautoban(name string, room *showdown.Room) {
	userID := showdown.ToID(name)
	user := bannedUser{User: userID, Room: room.ID}
	banlist.Lock()
	if _, ok := banlist.banlist[user]; ok {
		room.Reply("Unbanned")
		room.SendCommand("roomunban", string(userID))
		delete(banlist.banlist, user)
	} else {
		room.Reply("The user wasn't blacklisted?")
	}
	banlist.Unlock()
}

var chatCommands = map[string]func(string, *showdown.Room){
	"blacklist": autoban,
	"ban":       autoban,
	"ab":        autoban,
	"autoban":   autoban,

	"unblacklist": unautoban,
	"unban":       unautoban,
	"unab":        unautoban,
	"unautoban":   unautoban,
}

func commandCallback(command, argument string, room *showdown.Room) {
	if callback, ok := commands[command]; ok {
		callback(argument, room)
	}
}

func main() {
	var conf configurationOptions
	err := envconfig.Process("usainbot", &conf)
	if err != nil {
		log.Fatal(err)
	}
	err = initializeBlacklist()
	if err != nil {
		log.Fatal(err)
	}
	showdownConnection, connectionSuccess, err := showdown.ConnectToServer(showdown.LoginData{
		Nickname: conf.Nickname,
		Password: conf.Password,
		Rooms:    []string{"joim"},
	}, "showdown", commandCallback)
	if err != nil {
		log.Panic(err)
	}
	<-connectionSuccess
	log.Println("Connected")
	showdownConnection.SendGlobalCommand("userauth", "")
	for {
		time.Sleep(30 * time.Minute)
		var users []bannedUser
		banlist.RLock()
		for user := range banlist.banlist {
			users = append(users, user)
		}
		banlist.RUnlock()
		fileHandle, err := os.Create("blacklist.json")
		if err != nil {
			log.Fatal(err)
		}
		json.NewEncoder(fileHandle).Encode(users)
		fileHandle.Close()
	}
}
