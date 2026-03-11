package handler

const QueryParamCurrentPage string = "current_page"
const QueryParamUsername string = "username"

const FormFieldUsername string = QueryParamUsername
const FormFieldDisplayName string = "display_name"
const FormFieldAvatarURL string = "avatar_url"

type CtxSessionKey string

const (
	CtxSessionKeySessionId     CtxSessionKey = "sessionId"
	CtxSessionKeyUserIdFromJWT CtxSessionKey = "userIdFromJWT"
)
