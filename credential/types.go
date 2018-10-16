package credential

import "github.com/aws/aws-sdk-go/service/kms"

type PlaintextFile struct {
	content  []byte
	filePath string
}

// The fact KMS encryption produces different ciphertexts for the same plaintext had been
// causing unnecessary node replacements(https://github.com/kubernetes-incubator/kube-aws/issues/107)
// Persist encrypted assets for caching purpose so that we can avoid that.
type EncryptedFile struct {
	content             []byte
	filePath            string
	fingerprintFilePath string
	fingerprint         string
}

type CachedEncryptor struct {
	EncryptionService KMSEncryptionService
}

type EncryptionService interface {
	Encrypt(*kms.EncryptInput) (*kms.EncryptOutput, error)
}

type KMSEncryptionService struct {
	KmsKeyARN string
	KmsSvc    EncryptionService
}
