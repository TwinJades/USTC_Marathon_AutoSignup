package main

import (
	"errors"
	"fmt"
	"unicode/utf16"
	"encoding/binary"

	"github.com/danieljoos/wincred"
	"golang.org/x/sys/windows"
)

const credentialService = "USTC Marathon AutoSignup"

func deletePassword(username string) error {
	if username==""{return nil}
	for _,name:=range []string{credentialService,username+"@"+credentialService}{
		credential,err:=wincred.GetGenericCredential(name)
		if errors.Is(err,windows.ERROR_NOT_FOUND){continue}
		if err!=nil{return fmt.Errorf("Windows 凭据读取失败")}
		if credential.UserName!=username{continue}
		if err:=credential.Delete();err!=nil{return fmt.Errorf("Windows 凭据清除失败")}
	}
	return nil
}

func readPassword(username string) (string, error) {
	if username == "" { return "", nil }
	for _, name := range []string{credentialService, username + "@" + credentialService} {
		c, err := wincred.GetGenericCredential(name)
		if errors.Is(err, windows.ERROR_NOT_FOUND) { continue }
		if err != nil { return "", fmt.Errorf("Windows 凭据读取失败") }
		if c.UserName != username { continue }
		if len(c.CredentialBlob)%2 != 0 { return "", fmt.Errorf("Windows 凭据密码编码无效") }
		units := make([]uint16, len(c.CredentialBlob)/2)
		for i := range units { units[i] = binary.LittleEndian.Uint16(c.CredentialBlob[i*2:]) }
		return string(utf16.Decode(units)), nil
	}
	return "", nil
}

func writePassword(username, password string) error {
	if username == "" || password == "" { return fmt.Errorf("账户和密码不能为空") }
	c := wincred.NewGenericCredential(username + "@" + credentialService)
	c.UserName = username
	units := utf16.Encode([]rune(password))
	c.CredentialBlob = make([]byte, len(units)*2)
	for i, unit := range units { binary.LittleEndian.PutUint16(c.CredentialBlob[i*2:], unit) }
	c.Persist = wincred.PersistEnterprise
	// 更新同账号的旧凭据，避免旧密码优先于新密码。
	old, err := wincred.GetGenericCredential(credentialService)
	if err != nil && !errors.Is(err, windows.ERROR_NOT_FOUND) { return fmt.Errorf("Windows 凭据读取失败") }
	if err == nil && old.UserName == username {
		old.CredentialBlob = c.CredentialBlob
		if err := old.Write(); err != nil { return fmt.Errorf("Windows 凭据写入失败") }
	}
	if err := c.Write(); err != nil { return fmt.Errorf("Windows 凭据写入失败") }
	return nil
}
