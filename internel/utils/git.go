package utils

import (
	"fmt"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	ggssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-git/go-git/v5/storage/memory"
	"golang.org/x/crypto/ssh"
	"io"
)

// NewSSHAuth 创建一个基于 PEM 私钥字符串的 SSH 认证方法
func NewSSHAuth(sshKeyPEM string) (*ggssh.PublicKeys, error) {
	auth, err := ggssh.NewPublicKeys("git", []byte(sshKeyPEM), "")
	if err != nil {
		return nil, fmt.Errorf("create public keys: %w", err)
	}
	// WARNING: 不校验 host key（开发/测试用）。生产请替换为合适的 HostKeyCallback。
	auth.HostKeyCallbackHelper.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	return auth, nil
}

// CloneToMemory 克隆一个仓库到内存中
// depth: 克隆深度，0 表示完整克隆
// 修正：返回 billy.Filesystem 接口，而不是 *memfs.Memory
func CloneToMemory(repoURL string, auth transport.AuthMethod) (*git.Repository, billy.Filesystem, error) {
	storer := memory.NewStorage()
	fs := memfs.New() // fs 是 *memfs.Memory

	cloneOpts := &git.CloneOptions{
		URL:      repoURL,
		Auth:     auth,
		Progress: io.Discard,
	}

	repo, err := git.Clone(storer, fs, cloneOpts)
	if err != nil {
		return nil, nil, fmt.Errorf("clone: %w", err)
	}
	// 修正：返回 fs (*memfs.Memory) 作为 billy.Filesystem 接口
	return repo, fs, nil
}
