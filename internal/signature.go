package internal

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

func GenerateSignature(eoCltDvidn, ve, kp, requestParamValue, sacsft string, reqt int64) string {
	signText := eoCltDvidn + ve + kp + requestParamValue
	signSalt := fmt.Sprintf("%s:%d", sacsft, reqt)

	h := hmac.New(sha256.New, []byte(signSalt))
	h.Write([]byte(signText))
	signature := h.Sum(nil)

	return base64.StdEncoding.EncodeToString(signature)
}
