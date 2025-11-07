package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	ggssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"golang.org/x/crypto/ssh"
	"io"
	"os"
	"path/filepath"
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

// CloneOrUpdate 克隆或更新仓库到指定目录
func CloneOrUpdate(baseDir, repoURL string, auth transport.AuthMethod) (*git.Repository, billy.Filesystem, error) {
	// 用仓库地址计算 SHA256 作为文件夹名
	hash := sha256.Sum256([]byte(repoURL))
	folderName := hex.EncodeToString(hash[:])
	repoDir := filepath.Join(baseDir, folderName)

	fs := osfs.New(repoDir)

	var repo *git.Repository
	var err error

	if _, statErr := os.Stat(repoDir); os.IsNotExist(statErr) {
		// 目录不存在，执行克隆
		cloneOpts := &git.CloneOptions{
			URL:      repoURL,
			Auth:     auth,
			Progress: io.Discard,
		}

		repo, err = git.PlainClone(repoDir, false, cloneOpts)
		if err != nil {
			return nil, nil, fmt.Errorf("clone: %w", err)
		}
	} else {
		// 目录存在，尝试打开仓库
		repo, err = git.PlainOpen(repoDir)
		if err != nil {
			return nil, nil, fmt.Errorf("open existing repo: %w", err)
		}

		// 先 fetch
		err = repo.Fetch(&git.FetchOptions{
			RemoteName: "origin",
			Auth:       auth,
			Force:      true,
		})
		if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
			return nil, nil, fmt.Errorf("fetch: %w", err)
		}

		// 找到远程默认分支
		refs, err := repo.References()
		if err != nil {
			return nil, nil, fmt.Errorf("list references: %w", err)
		}

		var remoteRef *plumbing.Reference
		err = refs.ForEach(func(r *plumbing.Reference) error {
			if r.Name().IsRemote() && r.Name().String() != "" {
				remoteRef = r
				return errors.New("found") // 停止遍历
			}
			return nil
		})
		if remoteRef == nil {
			return nil, nil, fmt.Errorf("no remote branch found")
		}

		// 强制重置本地工作区到远程最新
		w, err := repo.Worktree()
		if err != nil {
			return nil, nil, fmt.Errorf("get worktree: %w", err)
		}

		err = w.Reset(&git.ResetOptions{
			Mode:   git.HardReset,
			Commit: remoteRef.Hash(),
		})
		if err != nil {
			return nil, nil, fmt.Errorf("reset: %w", err)
		}
	}

	return repo, fs, nil
}
