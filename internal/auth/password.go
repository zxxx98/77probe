package auth

import "github.com/alexedwards/argon2id"

const dummyPasswordHash = "$argon2id$v=19$m=65536,t=1,p=12$FDKiXYbml13FC/LuELea5Q$lLcuyKnc+IrVPHELeLyBVtPz872Q4x13pbmwguGktlg"

func hashPassword(password string) (string, error) {
	return argon2id.CreateHash(password, argon2id.DefaultParams)
}

func verifyPassword(password, encoded string) (bool, error) {
	return argon2id.ComparePasswordAndHash(password, encoded)
}
