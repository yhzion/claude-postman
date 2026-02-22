package config

// EmailPreset은 이메일 프로바이더별 호스트/포트 프리셋
type EmailPreset struct {
	SMTPHost string
	SMTPPort int
	IMAPHost string
	IMAPPort int
}

// Presets는 프로바이더별 이메일 프리셋 맵
var Presets = map[string]EmailPreset{
	"gmail": {
		SMTPHost: "smtp.gmail.com",
		SMTPPort: 587,
		IMAPHost: "imap.gmail.com",
		IMAPPort: 993,
	},
	"outlook": {
		SMTPHost: "smtp.office365.com",
		SMTPPort: 587,
		IMAPHost: "outlook.office365.com",
		IMAPPort: 993,
	},
}
