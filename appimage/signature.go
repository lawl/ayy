package appimage

import (
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/openpgp"
)

func (ai *AppImage) Signature() (signedby openpgp.Entity, ok bool, err error) {

	rawkey, err := ai.ELFSectionAsString(".sig_key")
	if err != nil {
		return openpgp.Entity{}, false, err
	}
	keyRingReader := strings.NewReader(rawkey)

	rawsig, err := ai.ELFSectionAsString(".sha256_sig")
	if err != nil {
		return openpgp.Entity{}, false, err
	}

	signature := strings.NewReader(rawsig)

	shabytes, err := ai.SHA256WithoutSignature()
	if err != nil {
		return openpgp.Entity{}, false, err
	}

	tocheck := fmt.Sprintf("%x", shabytes) // yes, really...

	keyring, err := openpgp.ReadArmoredKeyRing(keyRingReader)
	if err != nil {
		return openpgp.Entity{}, false, err
	}
	entity, err := openpgp.CheckArmoredDetachedSignature(keyring, strings.NewReader(tocheck), signature)
	if err != nil {
		return openpgp.Entity{}, false, err
	}

	return *entity, true, nil
}

// SHA256WithoutSignature computes the SHA256 sum of the AppImage
// but treats the ELF sections .sha256_sig and .sig_key as if
// they were entirely 0x00 bytes. That's because this hash
// is later signed and stuffed into exactly these sections
func (ai *AppImage) SHA256WithoutSignature() ([]byte, error) {
	if _, err := ai.file.Seek(0, io.SeekStart); err != nil {
		fmt.Println(err)
		return nil, err
	}
	hashTarget := NewSkipReader(ai.file)
	shasect := ai.elf.Section(".sha256_sig")
	if shasect != nil {
		hashTarget.AddSkip(shasect.Offset(), shasect.Length())
	}
	sigsect := ai.elf.Section(".sig_key")
	if sigsect != nil {
		hashTarget.AddSkip(sigsect.Offset(), sigsect.Length())
	}

	h := sha256.New()
	if _, err := ai.file.Seek(0, io.SeekStart); err != nil {
		fmt.Println(err)
		return nil, err
	}

	if _, err := io.Copy(h, hashTarget); err != nil {
		return nil, err
	}

	return h.Sum(nil), nil

}

func (ai *AppImage) SHA1WithoutSignature() ([]byte, error) {
	if _, err := ai.file.Seek(0, io.SeekStart); err != nil {
		fmt.Println(err)
		return nil, err
	}
	hashTarget := NewSkipReader(ai.file)
	shasect := ai.elf.Section(".sha256_sig")
	if shasect != nil {
		hashTarget.AddSkip(shasect.Offset(), shasect.Length())
	}
	sigsect := ai.elf.Section(".sig_key")
	if sigsect != nil {
		hashTarget.AddSkip(sigsect.Offset(), sigsect.Length())
	}

	h := sha1.New()
	if _, err := ai.file.Seek(0, io.SeekStart); err != nil {
		fmt.Println(err)
		return nil, err
	}

	if _, err := io.Copy(h, hashTarget); err != nil {
		return nil, err
	}

	return h.Sum(nil), nil

}

func (ai *AppImage) HasSignature() (ok bool) {
	rawkey, _ := ai.ELFSectionAsString(".sig_key")
	rawsig, _ := ai.ELFSectionAsString(".sha256_sig")
	return rawkey != "" && rawsig != ""
}
