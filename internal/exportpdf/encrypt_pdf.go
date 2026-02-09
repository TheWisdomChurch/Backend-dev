package exportpdf

import (
	"bytes"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// EncryptPDF encrypts a PDF using AES-256.
// password is used as both user + owner password so the same password opens it.
func EncryptPDF(pdfBytes []byte, password string) ([]byte, error) {
	pw := strings.TrimSpace(password)
	if pw == "" {
		pw = "changeme"
	}

	conf := model.NewAESConfiguration(pw, pw, 256)

	in := bytes.NewReader(pdfBytes)
	var out bytes.Buffer

	if err := api.Encrypt(in, &out, conf); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
