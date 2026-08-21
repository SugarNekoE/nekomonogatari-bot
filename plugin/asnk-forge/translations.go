package asnkforge

import "forge.asnk.io/sugar/nekomonogatari-bot/config"

type messages struct {
	commandText        string
	startButton        string
	claimRecorded      string
	openButton         string
	alreadyRegistered  string
	promptExpired      string
	registrationBusy   string
	internalError      string
	registrationNotice string

	web webMessages
}

type webMessages struct {
	Title               string `json:"title"`
	Subtitle            string `json:"subtitle"`
	LoginHeading        string `json:"loginHeading"`
	LoginInstructions   string `json:"loginInstructions"`
	NoPending           string `json:"noPending"`
	AlreadyRegistered   string `json:"alreadyRegistered"`
	SessionExpired      string `json:"sessionExpired"`
	FormHeading         string `json:"formHeading"`
	AuthenticatedAs     string `json:"authenticatedAs"`
	Username            string `json:"username"`
	UsernameHint        string `json:"usernameHint"`
	Email               string `json:"email"`
	Password            string `json:"password"`
	ConfirmPassword     string `json:"confirmPassword"`
	PasswordHint        string `json:"passwordHint"`
	PasswordMismatch    string `json:"passwordMismatch"`
	Submit              string `json:"submit"`
	Submitting          string `json:"submitting"`
	Success             string `json:"success"`
	Conflict            string `json:"conflict"`
	ValidationFailed    string `json:"validationFailed"`
	UpstreamFailed      string `json:"upstreamFailed"`
	InternalError       string `json:"internalError"`
	RetryLogin          string `json:"retryLogin"`
	NeverSharePassword  string `json:"neverSharePassword"`
	TelegramLoginFailed string `json:"telegramLoginFailed"`
	Loading             string `json:"loading"`
}

func messagesFor(language config.Language) messages {
	if language == config.LanguageChinese {
		return messages{
			commandText:        "Forgejo 账号注册\n\n点击下方按钮授权当前 Telegram 账号。机器人记录授权后，会在群聊中发送一个不含任何个人令牌的固定注册网址。请在网页使用同一个 Telegram 账号登录，并只在网页填写用户名、邮箱和密码。不要在群聊中发送这些信息。",
			startButton:        "授权此 Telegram 账号",
			claimRecorded:      "已记录 %s 的注册授权，有效期为 %s。请点击下方按钮，并在网页使用同一个 Telegram 账号登录。",
			openButton:         "打开 Forgejo 注册页面",
			alreadyRegistered:  "该 Telegram 账号已经注册过 Forgejo 账号。",
			promptExpired:      "此注册入口已过期，请重新执行 /forge。",
			registrationBusy:   "该 Telegram 账号已有正在处理的注册请求，请稍后再试。",
			internalError:      "暂时无法开始注册，请稍后重试。",
			registrationNotice: "%s 已成功创建 Forgejo 账号 %s。",
			web: webMessages{
				Title: "Forgejo 账号注册", Subtitle: "通过 Telegram 群聊授权安全创建账号",
				LoginHeading: "验证 Telegram 账号", LoginInstructions: "请使用刚才在群聊点击授权按钮的同一个 Telegram 账号登录。",
				NoPending:         "当前 Telegram 账号没有有效的群聊授权。请返回允许的群聊重新执行 /forge 并点击授权按钮。",
				AlreadyRegistered: "该 Telegram 账号已经注册过 Forgejo 账号。", SessionExpired: "登录会话已过期，请返回群聊重新授权。",
				FormHeading: "创建 Forgejo 账号", AuthenticatedAs: "已验证 Telegram 账号：",
				Username: "用户名", UsernameHint: "1–40 个字符，只能使用英文字母、数字、点、下划线和连字符。",
				Email: "邮箱", Password: "密码", ConfirmPassword: "确认密码", PasswordHint: "至少 8 个字符。",
				PasswordMismatch: "两次输入的密码不一致。", Submit: "创建账号", Submitting: "正在创建…", Success: "Forgejo 账号创建成功。",
				Conflict: "用户名或邮箱已被使用，请修改后重试。", ValidationFailed: "请检查用户名、邮箱和密码。",
				UpstreamFailed: "Forgejo 暂时不可用。为避免重复创建，请联系管理员后再重试。", InternalError: "服务器发生错误，请稍后重试。",
				RetryLogin: "重新验证", NeverSharePassword: "密码只会发送到 Forgejo，不会被机器人保存。请勿在群聊中分享密码。",
				TelegramLoginFailed: "Telegram 登录验证失败。", Loading: "正在加载…",
			},
		}
	}

	return messages{
		commandText:        "Forgejo account registration\n\nTap the button below to authorize this Telegram account. The bot will then post a constant registration link with no personal token. Sign in on the website with the same Telegram account and enter your username, email, and password only there. Never post those details in this group.",
		startButton:        "Authorize this Telegram account",
		claimRecorded:      "Registration access was recorded for %s and expires in %s. Open the page below and sign in with this same Telegram account.",
		openButton:         "Open Forgejo registration",
		alreadyRegistered:  "This Telegram account has already registered a Forgejo account.",
		promptExpired:      "This registration prompt has expired. Run /forge again.",
		registrationBusy:   "A registration for this Telegram account is already being processed. Try again shortly.",
		internalError:      "Registration cannot be started right now. Please try again later.",
		registrationNotice: "%s successfully created the Forgejo account %s.",
		web: webMessages{
			Title: "Forgejo account registration", Subtitle: "Create an account securely after authorization in Telegram",
			LoginHeading: "Verify your Telegram account", LoginInstructions: "Sign in with the same Telegram account that tapped the authorization button in the group.",
			NoPending:         "This Telegram account has no active group authorization. Return to an allowed group, run /forge, and tap the authorization button.",
			AlreadyRegistered: "This Telegram account has already registered a Forgejo account.", SessionExpired: "Your session expired. Return to the group and authorize again.",
			FormHeading: "Create your Forgejo account", AuthenticatedAs: "Verified Telegram account:",
			Username: "Username", UsernameHint: "1–40 characters using letters, numbers, dots, underscores, or hyphens.",
			Email: "Email", Password: "Password", ConfirmPassword: "Confirm password", PasswordHint: "Use at least 8 characters.",
			PasswordMismatch: "The passwords do not match.", Submit: "Create account", Submitting: "Creating…", Success: "Your Forgejo account was created.",
			Conflict: "That username or email is already in use. Change it and try again.", ValidationFailed: "Check the username, email, and password fields.",
			UpstreamFailed: "Forgejo is temporarily unavailable. To avoid a duplicate account, contact an administrator before retrying.", InternalError: "The server encountered an error. Please try again later.",
			RetryLogin: "Verify again", NeverSharePassword: "The bot never stores your password; it is sent only to Forgejo. Never share it in the group.",
			TelegramLoginFailed: "Telegram login verification failed.", Loading: "Loading…",
		},
	}
}
