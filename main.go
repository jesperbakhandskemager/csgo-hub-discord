package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/bwmarrin/discordgo"
	"gopkg.in/yaml.v3"
)

const API string = "https://steam.csgohub.xyz/"

var BotID string

var s *discordgo.Session

type YAMLFile struct {
	Config Config `yaml:"config"`
}

type Config struct {
	STEAM_KEY     string `yaml:"STEAM_KEY"`
	DISCORD_TOKEN string `yaml:"DISCORD_TOKEN"`
}

func ReadConfig() (*Config, error) {
	config := &YAMLFile{}
	cfgFile, err := os.ReadFile("./config.yaml")
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(cfgFile, config)
	return &config.Config, err
}

var (
	GuildID        = flag.String("guild", "", "Test guild ID. If not passed - bot registers commands globally")
	RemoveCommands = flag.Bool("rmcmd", true, "Remove all commands after shutdowning or not")
)

func init() {
	config, err := ReadConfig()
	if err != nil {
		panic(err)
	}
	s, err = discordgo.New("Bot " + config.DISCORD_TOKEN)
	if err != nil {
		log.Fatalf("Invalid bot parameters: %v", err)
	}
	u, err := s.User("@me")
	if err != nil {
		log.Fatal(err)
		return
	}

	BotID = u.ID
}

var (
	commands = []*discordgo.ApplicationCommand{
		{
			Name:        "link-steam",
			Description: "Link your steam account",
			Type:        discordgo.ChatApplicationCommand,
		},
		{
			Name:        "show-team",
			Description: "Shows the CS:GO friend codes of the other users in VC",
			Type:        discordgo.ChatApplicationCommand,
		},
	}

	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"link-steam": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			if i.Member == nil {
				token := GetToken(i.User.ID)

				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "Your verification link:",
					},
				})
				s.ChannelMessageSend(i.ChannelID, API+token)
				return
			}
			sender := i.Member.User.ID
			token := GetToken(sender)
			userDM, err := s.UserChannelCreate(sender)
			if err != nil {
				log.Fatal(err)
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "Couldn't send you verification link",
					},
				})
				return
			}
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "A verification link has been send to your DM's",
				},
			})
			s.ChannelMessageSend(userDM.ID, API+token)

		},
		"show-team": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			// Find the guild for that channel.
			g, err := s.State.Guild(i.GuildID)
			if err != nil {
				// Could not find guild.
				log.Println("couldn't find guild")
				return
			}

			var vcChannelId string
			var vcUsers []string

			// Look for the message sender in that guild's current voice states.
			for _, vs := range g.VoiceStates {
				if vs.UserID == i.Member.User.ID {
					vcChannelId = vs.ChannelID
				}
			}

			for _, vs := range g.VoiceStates {
				if vs.ChannelID == vcChannelId {
					vcUsers = append(vcUsers, vs.UserID)
				}
			}
			var returnString strings.Builder
			friends := GetFriendCodes(vcUsers)
			for _, friend := range friends {
				if friend.DiscordId != "" {
					returnString.WriteString("<@" + friend.DiscordId + ">: " + friend.FriendCode + "\n")
				}
			}
			if returnString.String() == "" {
				returnString.WriteString("No friend codes found")
			}

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: returnString.String(),
				},
			})
		},
	}
)

type userStruct struct {
	Id         int    `json:"id"`
	CreatedAt  string `json:"created_at"`
	DiscordId  string `json:"discord_id"`
	FriendCode string `json:"friend_code"`
}

func GetFriendCode(users []string) []userStruct {
	var userArr []userStruct
	for _, u := range users {
		resp, err := http.Get("http://localhost:8383/api/v1/user/" + u)
		if err != nil {
			log.Println(err)
			log.Println("quiting")
			return userArr
		}
		var userTemp userStruct
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		json.Unmarshal(bodyBytes, &userTemp)
		userArr = append(userArr, userTemp)
	}
	return userArr
}

func GetFriendCodes(users []string) []userStruct {
	var userArr []userStruct

	for _, u := range users {
		var us userStruct
		us.DiscordId = u
		userArr = append(userArr, us)
	}
	jsonReq, err := json.Marshal(userArr)
	if err != nil {
		log.Println(err)
		return userArr
	}
	userArr = nil

	resp, err := http.Post("http://localhost:8383/api/v1/users", "application/json; charset=utf-8", bytes.NewBuffer(jsonReq))
	if err != nil {
		log.Fatalln(err)
	}

	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	json.Unmarshal(bodyBytes, &userArr)

	return userArr
}

func GetToken(discordId string) string {
	resp, err := http.Get("http://localhost:8383/api/v1/token/" + discordId)
	if err != nil {
		log.Println(err)
		return ""
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyString := strings.Replace(string(bodyBytes), "\"", "", -1)
	println(bodyString)
	return bodyString
}

func main() {
	s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) { log.Println("Bot is up!") })
	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
	})
	s.Identify.Intents = discordgo.IntentsGuildVoiceStates
	err := s.Open()
	if err != nil {
		log.Fatalf("Cannot open the session: %v", err)
	}
	defer s.Close()

	createdCommands, err := s.ApplicationCommandBulkOverwrite(s.State.User.ID, *GuildID, commands)

	if err != nil {
		log.Fatalf("Cannot register commands: %v", err)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
	log.Println("Gracefully shutting down")

	if *RemoveCommands {
		for _, cmd := range createdCommands {
			err := s.ApplicationCommandDelete(s.State.User.ID, *GuildID, cmd.ID)
			if err != nil {
				log.Fatalf("Cannot delete %q command: %v", cmd.Name, err)
			}
		}
	}
}
