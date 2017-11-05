package main

import (
	"bytes"
	"encoding/gob"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/dchest/captcha"
	"golang.org/x/image/draw"
)

const (
	width                 int   = 300
	height                int   = 300
	cooldown_time         int64 = 60
	captcha_cooldown_time int64 = 120
)

var (
	cooldowns         map[string]int64
	captcha_cooldowns map[string]int64
	captcha_answers   map[string]string
	info              [(width + 1) * (height + 1)]string //TODO arrays of arrays.
	count             [(width + 1) * (height + 1)]uint64
	img               *image.RGBA
	changed_flag      bool
	running           bool
	post_id           string
	master_server     string
	token             string
	captcha_on        bool
)

func init() {
	flag.StringVar(&token, "t", "", "Bot Token")
	flag.StringVar(&master_server, "s", "", "Server ID")
	flag.BoolVar(&captcha_on, "c", false, "Enable captcha")
	flag.Parse()

	captcha_cooldowns = make(map[string]int64)
	captcha_answers = make(map[string]string)
	cooldowns = make(map[string]int64)

	blank()

	rand.Seed(time.Now().UnixNano())

	mass_load()

	go func() { // Autosave goroutine.
		for {
			time.Sleep(10 * time.Minute)
			mass_save()
			fmt.Println("Saving")
		}

	}()
}

func mass_load() {
	load("info", &info)
	load("count", &count)
	load("img", img)
	load("post_id", &post_id)
}

func mass_save() {
	save("info", info)
	save("count", count)
	save("img", img)
	save("post_id", post_id)
}

func blank() {
	img = image.NewRGBA(image.Rect(0, 0, (width + 1), (height + 1)))

	for ix := 0; ix <= (width + 1); ix++ {
		for iy := 0; iy <= (height + 1); iy++ {
			img.Set(ix, iy, color.RGBA{255, 255, 255, 255})
		}
	}
}

func save(name string, data interface{}) {

	file, err := os.Create(name + ".gob")
	defer file.Close()

	if err == nil {
		gob.NewEncoder(file).Encode(data)
	} else {
		fmt.Println(err)
	}
}

func load(name string, data interface{}) {
	file, err := os.Open(name + ".gob")
	defer file.Close()

	if err == nil {
		gob.NewDecoder(file).Decode(data)
	} else {
		fmt.Println(err)
	}
}

func main() {

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		fmt.Println("Error creating Discord session:", err)
		return
	}

	dg.AddHandler(messageCreate)
	dg.AddHandler(ready)
	dg.AddHandler(userHere)

	err = dg.Open()
	if err != nil {
		fmt.Println("Error opening connection:", err)
		return
	}

	fmt.Println("Bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	dg.Close()
}

func ready(s *discordgo.Session, event *discordgo.Ready) {
	s.UpdateStatus(0, "Plotting pixels")
	if post_id != "" && running == false {
		running = true
		fmt.Println("Starting off at", post_id)
		go image_loop(s)
	}
}

func userHere(s *discordgo.Session, evt *discordgo.PresenceUpdate) {
	guild, _ := s.Guild(master_server)
	member, err := s.GuildMember(evt.GuildID, evt.User.ID)

	if err == nil {
		if evt.Status == "online" || evt.Status == "dnd" || evt.Status == "away" {
			if len(member.Roles) == 0 {
				grank := guild.Roles[rand.Intn(len(guild.Roles)-1)]
				for grank.Permissions&discordgo.PermissionAdministrator == discordgo.PermissionAdministrator {
					grank = guild.Roles[rand.Intn(len(guild.Roles)-1)]
				}

				red := uint8(grank.Color >> 16)
				green := uint8((grank.Color & 0x00FFFF) >> 8)
				blue := uint8((grank.Color & 0x0000FF))

				// If they have pixels under another colour, change them.
				for i, j := range info {
					if j == member.User.ID {
						x := i / width
						y := i - (width * x)

						fmt.Println("Replacing", x, y)
						img.Set(x, y, color.RGBA{red, green, blue, 255})
					}

				}

				s.GuildMemberRoleAdd(evt.GuildID, evt.User.ID, grank.ID)
			}
		}
	}

}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	achan, _ := s.Channel(m.ChannelID)
	guild, _ := s.Guild(achan.GuildID)
	member, _ := s.GuildMember(achan.GuildID, m.Author.ID)

	admin := false

	for _, role := range guild.Roles {
		for _, roleID := range member.Roles {
			if roleID == role.ID && role.Permissions&discordgo.PermissionAdministrator == discordgo.PermissionAdministrator {
				admin = true
			}

		}
	}

	parts := strings.Split(m.Content, " ")

	if m.Author.ID == s.State.User.ID || len(parts) < 1 {
		return
	}

	if parts[0] == "$info" && len(parts) == 3 {
		x, err := strconv.Atoi(parts[1])
		y, err2 := strconv.Atoi(parts[2])

		if err == nil && err2 == nil {
			if x <= width && x >= 0 && y <= height && y >= 0 {
				if info[(width*x)+y] != "" {
					msg := " by <@" + info[(width*x)+y] + ">"

					ared, agreen, ablue, _ := img.At((x), (height - y)).RGBA()

					trigger := false

					for _, role := range guild.Roles {

						red := uint8(role.Color >> 16)
						green := uint8((role.Color & 0x00FFFF) >> 8)
						blue := uint8((role.Color & 0x0000FF))

						if uint8(ared) == red && uint8(ablue) == blue && uint8(agreen) == green {

							msg += " (" + role.Name + ")"
							trigger = true
							break

						}
					}

					if trigger {
						msg = "Placed" + msg
					} else {
						msg = "Removed" + msg
					}

					s.ChannelMessageSend(m.ChannelID, msg+" ,overall the pixel has been placed "+strconv.FormatUint(count[(width*x)+y], 10)+" times.")
				} else {
					s.ChannelMessageSend(m.ChannelID, "Could not get pixel info.")
				}
			} else {
				s.ChannelMessageSend(m.ChannelID, "Out of range.")
			}

		}

	}

	if parts[0] == "$zoom" && len(parts) == 3 {
		x, err := strconv.Atoi(parts[1])
		y, err2 := strconv.Atoi(parts[2])

		if err == nil && err2 == nil {
			if x <= width && x >= 0 && y <= height && y >= 0 {
				newimg := scale_image(img.SubImage(image.Rectangle{image.Point{x - 9, (height - y) - 9}, image.Point{x + 10, (height - y) + 10}}), 10)
				post_image(s, newimg, m.ChannelID)
			} else {
				s.ChannelMessageSend(m.ChannelID, "Out of range.")
			}

		} else {
			s.ChannelMessageSend(m.ChannelID, "Wrong format.")
		}
	}

	if parts[0] == "$captcha" && captcha_on {
		if len(parts) > 1 && captcha_answers[m.Author.ID] != "" {
			if captcha_answers[m.Author.ID] == parts[1] {
				captcha_cooldowns[m.Author.ID] = time.Now().Unix()
				s.ChannelMessageSend(m.ChannelID, "Correct.")
			} else {
				s.ChannelMessageSend(m.ChannelID, "Incorrect.")
			}
		} else {
			gen_captcha(s, m)

		}

	}

	if (parts[0] == "$plot" || parts[0] == "$" || parts[0] == "$remove") && len(parts) > 2 {

		if time.Now().Unix()-captcha_cooldowns[m.Author.ID] <= captcha_cooldown_time || !captcha_on {
			if time.Now().Unix()-cooldowns[m.Author.ID] >= cooldown_time {
				x, err := strconv.Atoi(parts[1])
				y, err2 := strconv.Atoi(parts[2])

				if err == nil && err2 == nil {
					if x <= width && x >= 0 && y <= height && y >= 0 {
						red := uint8(255)
						green := uint8(255)
						blue := uint8(255)
						trigger := false

						if parts[0] == "$remove" {
							trigger = true
						} else {
							for _, role := range guild.Roles {
								for _, roleID := range member.Roles {
									if roleID == role.ID && role.Permissions&discordgo.PermissionAdministrator != discordgo.PermissionAdministrator {
										red = uint8(role.Color >> 16)
										green = uint8((role.Color & 0x00FFFF) >> 8)
										blue = uint8((role.Color & 0x0000FF))
										trigger = true
									}

								}
							}
						}

						if trigger {
							dot_colour := color.RGBA{red, green, blue, 255}
							if dot_colour != img.At(x, (height-y)) {
								s.ChannelMessageSend(m.ChannelID, "Placing at "+parts[1]+"x"+parts[2])
								cooldowns[m.Author.ID] = time.Now().Unix()
								info[(width*x)+y] = m.Author.ID
								img.Set(x, (height - y), dot_colour)
								newimg := scale_image(img.SubImage(image.Rectangle{image.Point{x - 14, (height - y) - 14}, image.Point{x + 15, (height - y) + 15}}), 10)
								post_image(s, newimg, m.ChannelID)
								changed_flag = true
								count[(width*x)+y]++

								if captcha_cooldowns[m.Author.ID] == 0 {
									captcha_cooldowns[m.Author.ID] = time.Now().Unix()
								}

							} else {
								s.ChannelMessageSend(m.ChannelID, "That wouldn't make a difference.")
							}

						} else {
							s.ChannelMessageSend(m.ChannelID, "You need a rank.")
						}

					} else {
						s.ChannelMessageSend(m.ChannelID, "Out of range.")
					}
				} else {
					s.ChannelMessageSend(m.ChannelID, "Wrong format.")
				}
			} else {
				s.ChannelMessageSend(m.ChannelID, "Please wait "+strconv.Itoa(int(cooldown_time-(time.Now().Unix()-cooldowns[m.Author.ID])))+" seconds.")
			}
		} else {
			s.ChannelMessageSend(m.ChannelID, "You need to redo the captcha (`$captcha 12345`or `$captcha` by its self to get another one)")
			gen_captcha(s, m)
		}

	}

	if m.Content == "$post" && admin {
		post_id = m.ChannelID
		save("post_id", post_id)
		running = false
		s.ChannelMessageSend(m.ChannelID, "Will post images here from now on.")
		running = true
		go image_loop(s)
	}

	if m.Content == "$clear" && admin {
		blank()
		s.ChannelMessageSend(m.ChannelID, "Clearing...")
	}

	if m.Content == "$image" && admin {
		post_image(s, img, m.ChannelID)
	}

	if m.Content == "$load" && admin {
		mass_load()
		s.ChannelMessageSend(m.ChannelID, "Loading...")
	}

	if m.Content == "$save" && admin {
		mass_save()
		s.ChannelMessageSend(m.ChannelID, "Saving...")
	}
}

func image_loop(s *discordgo.Session) {
	for running {
		time.Sleep(15 * time.Minute)
		if changed_flag {
			post_image(s, img, post_id)
			changed_flag = false
		}

	}
}

func post_image(s *discordgo.Session, src image.Image, id string) {

	buf := new(bytes.Buffer)

	png.Encode(buf, src)

	r := bytes.NewReader(buf.Bytes())
	s.ChannelFileSend(id, "place.png", r)
}

func scale_image(src image.Image, scale int) *image.RGBA {
	newimg := image.NewRGBA(image.Rect(0, 0, src.Bounds().Dx()*scale, src.Bounds().Dy()*scale))

	draw.NearestNeighbor.Scale(newimg, newimg.Bounds(), src, src.Bounds(), draw.Over, nil)

	return newimg
}

func gen_captcha(s *discordgo.Session, m *discordgo.MessageCreate) {
	ans := captcha.RandomDigits(5)
	post_image(s, captcha.NewImage("", ans, 240, 80), m.ChannelID)
	captcha_answers[m.Author.ID] = ""

	for _, j := range ans {
		captcha_answers[m.Author.ID] += strconv.FormatUint(uint64(j), 10)
	}
}
