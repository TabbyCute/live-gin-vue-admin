package anchorhome

import "errors"

var (
	ErrInvalid     = errors.New("invalid")
	ErrForbidden   = errors.New("forbidden")
	ErrNotFound    = errors.New("not_found")
	ErrVersion     = errors.New("version_conflict")
	ErrIdempotency = errors.New("idempotency_conflict")
	ErrQuota       = errors.New("quota")
	ErrUploadLimit = errors.New("upload_limit")
	ErrBusy        = errors.New("busy")
	ErrUnavailable = errors.New("unavailable")
)

func ErrorCode(err error) string {
	for _, known := range []error{ErrInvalid, ErrForbidden, ErrNotFound, ErrVersion, ErrIdempotency, ErrQuota, ErrUploadLimit, ErrBusy, ErrUnavailable} {
		if errors.Is(err, known) {
			return known.Error()
		}
	}
	return ErrUnavailable.Error()
}
func PublicError(err error) string {
	switch ErrorCode(err) {
	case "invalid":
		return "请求参数或图片格式无效"
	case "forbidden":
		return "无权进行此操作"
	case "not_found":
		return "内容不存在或不可见"
	case "version_conflict":
		return "内容已变更，请刷新后重试"
	case "idempotency_conflict":
		return "请求编号已用于其他操作"
	case "quota":
		return "相册已达到图片数量上限"
	case "upload_limit":
		return "照片上传次数已达当日上限，请稍后再试"
	case "busy":
		return "图片处理繁忙，请稍后重试"
	default:
		return "服务暂不可用，请稍后重试"
	}
}
