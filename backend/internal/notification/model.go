package notification

type Channel string

const (
	ChannelInApp  Channel = "in_app"
	ChannelEmail  Channel = "email"
	ChannelWechat Channel = "wechat"
)

type Message struct {
	ID      uint64
	UserID  uint64
	Channel Channel
	Title   string
	Body    string
}
