package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mixgram-core/internel/utils"
	"os"
	"time"

	git "github.com/go-git/go-git/v5"
	ggconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

var (
	UserName  = "MixGram"
	UserEmail = "admin@mixgram.org"
)

// PushCommit 用 ssh 私钥字符串向远端仓库提交并推送一个 commit。
func PushCommit(repoURL, sshKeyPEM string, commitMsg string) error {
	// 1) 准备 auth
	auth, err := utils.NewSSHAuth(sshKeyPEM)
	if err != nil {
		return err
	}
	files := map[string][]byte{
		"README.MD": []byte(utils.RandomHexString(32)),
	}

	// 2) 克隆到内存 (完整克隆, depth=0)
	// 修正：我们不再需要 clone 返回的 fs，用 _ 忽略
	repo, _, err := utils.CloneToMemory(repoURL, auth)
	if err != nil {
		return fmt.Errorf("clone repo: %w", err)
	}

	// 3) 工作区（worktree）
	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("worktree: %w", err)
	}

	// 3.5) 获取当前分支引用
	headRef, err := repo.Head()
	if err != nil {
		return fmt.Errorf("head: %w", err)
	}
	refName := headRef.Name()
	if !refName.IsBranch() {
		return fmt.Errorf("HEAD is not on a branch: %s", refName.String())
	}

	// 4) 写入/修改文件到内存 fs
	// 关键修正：使用 wt.Filesystem 来操作文件，这是 go-git 的标准方式
	for path, content := range files {
		f, err := wt.Filesystem.Create(path)
		if err != nil {
			// 如果父目录不存在，Create 会在需要时创建目录。若失败则返回。
			return fmt.Errorf("create file %s: %w", path, err)
		}
		_, _ = f.Write(content)
		_ = f.Close()
		// git add
		_, err = wt.Add(path)
		if err != nil {
			return fmt.Errorf("add %s: %w", path, err)
		}
	}

	// 5) commit
	_, err = wt.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  UserName,
			Email: UserEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// 6) push to origin
	pushOpts := &git.PushOptions{
		Auth: auth,
		RefSpecs: []ggconfig.RefSpec{
			// 优化：明确推送当前分支，而不是 "refs/heads/*"
			ggconfig.RefSpec(fmt.Sprintf("%s:%s", refName, refName)),
		},
		Progress: os.Stdout,
	}
	if err := repo.Push(pushOpts); err != nil {
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			return nil
		}
		return fmt.Errorf("push: %w", err)
	}

	return nil
}

// SimpleCommit 描述一个简化的 commit 信息
type SimpleCommit struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Email   string `json:"email"`
	Message string `json:"message"`
	Date    int64  `json:"date"`
}

func FetchCommitsJSON(repoURL, sshKeyPEM string, max int) (string, error) {
	commits, err := FetchCommits(repoURL, sshKeyPEM, max)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(commits)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FetchCommits 克隆远端并列出最近的 N 条 commit（返回 commit 信息数组）
func FetchCommits(repoURL, sshKeyPEM string, max int) ([]SimpleCommit, error) {
	auth, err := utils.NewSSHAuth(sshKeyPEM)
	if err != nil {
		return nil, err
	}

	// 修正：我们不需要 fs，所以用 _ 忽略
	repo, _, err := utils.CloneToMemory(repoURL, auth)
	if err != nil {
		return nil, err
	}

	// 获取 HEAD 引用
	ref, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("head: %w", err)
	}

	cIter, err := repo.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		return nil, fmt.Errorf("log: %w", err)
	}
	defer cIter.Close()

	results := make([]SimpleCommit, 0, max)
	count := 0
	err = cIter.ForEach(func(c *object.Commit) error {
		if max > 0 && count >= max {
			return io.EOF // 结束遍历
		}
		results = append(results, SimpleCommit{
			Hash:    c.Hash.String(),
			Author:  c.Author.Name,
			Email:   c.Author.Email,
			Message: c.Message,
			Date:    c.Author.When.UnixMilli(),
		})
		count++
		return nil
	})
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("iterate log: %w", err)
	}
	return results, nil
}

// TrimOldCommits 重写远端仓库历史，只保留最近的 keep 条 commit
func TrimOldCommits(repoURL, sshKeyPEM string, keep int) error {
	auth, err := utils.NewSSHAuth(sshKeyPEM)
	if err != nil {
		return err
	}

	// 修正：我们不需要 fs，所以用 _ 忽略
	repo, _, err := utils.CloneToMemory(repoURL, auth)
	if err != nil {
		return err
	}

	headRef, err := repo.Head()
	if err != nil {
		return fmt.Errorf("head: %w", err)
	}
	refName := headRef.Name()
	if !refName.IsBranch() {
		return fmt.Errorf("HEAD is not on a branch: %s", refName.String())
	}

	iter, err := repo.Log(&git.LogOptions{From: headRef.Hash()})
	if err != nil {
		return fmt.Errorf("log: %w", err)
	}
	defer iter.Close()

	var commits []*object.Commit
	_ = iter.ForEach(func(c *object.Commit) error {
		commits = append(commits, c)
		return nil
	})

	if len(commits) <= keep {
		fmt.Printf("commit 总数 %d <= %d，无需裁剪\n", len(commits), keep)
		return nil
	}

	// -----------------------------------------------------------------
	// 核心修改逻辑：重写历史
	// -----------------------------------------------------------------
	newRootAncestor := commits[keep-1]
	tree, err := newRootAncestor.Tree()
	if err != nil {
		return fmt.Errorf("get tree for new root: %w", err)
	}

	storer := repo.Storer
	newRootCommit := &object.Commit{
		Author:       newRootAncestor.Author,
		Committer:    object.Signature{Name: UserName, Email: UserEmail, When: time.Now()},
		Message:      newRootAncestor.Message,
		TreeHash:     tree.Hash,
		ParentHashes: []plumbing.Hash{},
	}

	obj := storer.NewEncodedObject()
	if err := newRootCommit.Encode(obj); err != nil {
		return fmt.Errorf("encode new root commit: %w", err)
	}
	newRootHash, err := storer.SetEncodedObject(obj)
	if err != nil {
		return fmt.Errorf("store new root commit: %w", err)
	}

	currentParentHash := newRootHash

	for i := keep - 2; i >= 0; i-- {
		oldCommit := commits[i]
		oldTree, err := oldCommit.Tree()
		if err != nil {
			return fmt.Errorf("get tree for commit %s: %w", oldCommit.Hash.String(), err)
		}

		newCommit := &object.Commit{
			Author:       oldCommit.Author,
			Committer:    object.Signature{Name: UserName, Email: UserEmail, When: time.Now()},
			Message:      oldCommit.Message,
			TreeHash:     oldTree.Hash,
			ParentHashes: []plumbing.Hash{currentParentHash},
		}

		obj := storer.NewEncodedObject()
		if err := newCommit.Encode(obj); err != nil {
			return fmt.Errorf("encode rebased commit: %w", err)
		}
		newCommitHash, err := storer.SetEncodedObject(obj)
		if err != nil {
			return fmt.Errorf("store rebased commit: %w", err)
		}
		currentParentHash = newCommitHash
	}

	finalHeadHash := currentParentHash
	mainRef := plumbing.NewHashReference(refName, finalHeadHash)
	if err := repo.Storer.SetReference(mainRef); err != nil {
		return fmt.Errorf("set ref: %w", err)
	}

	err = repo.Push(&git.PushOptions{
		Auth:  auth,
		Force: true,
		RefSpecs: []ggconfig.RefSpec{
			ggconfig.RefSpec(fmt.Sprintf("%s:%s", refName, refName)),
		},
		Progress: io.Discard,
	})
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}

	fmt.Printf("成功裁剪：保留最近 %d 条 commit，共删除 %d 条\n", keep, len(commits)-keep)
	return nil
}

// DeleteCommit 通过哈希值删除远端仓库历史中的一个 commit，并强制推送。
// 此操作会重写历史记录。
func DeleteCommit(repoURL, sshKeyPEM string, commitHash string) error {
	auth, err := utils.NewSSHAuth(sshKeyPEM)
	if err != nil {
		return err
	}

	// 克隆到内存 (完整克隆, depth=0)
	repo, _, err := utils.CloneToMemory(repoURL, auth)
	if err != nil {
		return fmt.Errorf("clone repo: %w", err)
	}

	// 获取当前分支引用
	headRef, err := repo.Head()
	if err != nil {
		return fmt.Errorf("head: %w", err)
	}
	refName := headRef.Name()
	if !refName.IsBranch() {
		return fmt.Errorf("HEAD is not on a branch: %s", refName.String())
	}

	// 遍历日志，收集所有 commit 并找到目标索引
	iter, err := repo.Log(&git.LogOptions{From: headRef.Hash()})
	if err != nil {
		return fmt.Errorf("log: %w", err)
	}
	defer iter.Close()

	var commits []*object.Commit // HEAD -> ... -> Root
	var targetIndex = -1
	targetHash := plumbing.NewHash(commitHash)

	_ = iter.ForEach(func(c *object.Commit) error {
		if c.Hash == targetHash {
			targetIndex = len(commits)
		}
		commits = append(commits, c)
		return nil
	})

	if targetIndex == -1 {
		return errors.New("commit not found in history")
	}
	if len(commits) == 1 {
		return errors.New("cannot delete the only commit in the repository")
	}

	// 准备新的 commit 列表 (Root -> ... -> New HEAD)，跳过被删除的目标
	var newCommits []*object.Commit
	for i := len(commits) - 1; i >= 0; i-- {
		if i != targetIndex {
			newCommits = append(newCommits, commits[i])
		}
	}

	// 核心修改逻辑：重建历史链条
	storer := repo.Storer
	var currentParentHash plumbing.Hash

	for i, oldCommit := range newCommits {
		oldTree, err := oldCommit.Tree()
		if err != nil {
			return fmt.Errorf("get tree for commit %s: %w", oldCommit.Hash.String(), err)
		}

		parentHashes := []plumbing.Hash{}
		if i > 0 { // 非根提交
			parentHashes = []plumbing.Hash{currentParentHash}
		}

		// 创建新的 commit 对象 (保留原作者信息，用 MixGram 作为 Committer)
		newCommit := &object.Commit{
			Author:       oldCommit.Author,
			Committer:    object.Signature{Name: UserName, Email: UserEmail, When: time.Now()}, // 使用新的 Committer 和时间
			Message:      oldCommit.Message,
			TreeHash:     oldTree.Hash,
			ParentHashes: parentHashes,
		}

		obj := storer.NewEncodedObject()
		if err := newCommit.Encode(obj); err != nil {
			return fmt.Errorf("encode rebased commit: %w", err)
		}
		currentParentHash, err = storer.SetEncodedObject(obj)
		if err != nil {
			return fmt.Errorf("store rebased commit: %w", err)
		}
	}

	// 设置新的引用
	finalHeadHash := currentParentHash
	mainRef := plumbing.NewHashReference(refName, finalHeadHash)
	if err := repo.Storer.SetReference(mainRef); err != nil {
		return fmt.Errorf("set ref: %w", err)
	}

	// 强制推送
	err = repo.Push(&git.PushOptions{
		Auth:  auth,
		Force: true,
		RefSpecs: []ggconfig.RefSpec{
			ggconfig.RefSpec(fmt.Sprintf("%s:%s", refName, refName)),
		},
		Progress: io.Discard,
	})
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}

	fmt.Printf("成功删除 commit %s，并重写历史\n", commitHash)
	return nil
}

// ModifyCommit 通过哈希值修改远端仓库历史中一个 commit 的提交信息，并强制推送。
// 此操作会重写历史记录。
func ModifyCommit(repoURL, sshKeyPEM string, commitHash string, newCommitMsg string) error {
	auth, err := utils.NewSSHAuth(sshKeyPEM)
	if err != nil {
		return err
	}

	// 克隆到内存 (完整克隆, depth=0)
	repo, _, err := utils.CloneToMemory(repoURL, auth)
	if err != nil {
		return fmt.Errorf("clone repo: %w", err)
	}

	// 获取当前分支引用
	headRef, err := repo.Head()
	if err != nil {
		return fmt.Errorf("head: %w", err)
	}
	refName := headRef.Name()
	if !refName.IsBranch() {
		return fmt.Errorf("HEAD is not on a branch: %s", refName.String())
	}

	// 遍历日志，收集所有 commit
	iter, err := repo.Log(&git.LogOptions{From: headRef.Hash()})
	if err != nil {
		return fmt.Errorf("log: %w", err)
	}
	defer iter.Close()

	var commits []*object.Commit // HEAD -> ... -> Root
	targetHash := plumbing.NewHash(commitHash)
	foundTarget := false

	_ = iter.ForEach(func(c *object.Commit) error {
		if c.Hash == targetHash {
			foundTarget = true
		}
		commits = append(commits, c)
		return nil
	})

	if !foundTarget {
		return errors.New("commit not found in history")
	}

	// 反转列表 (Root -> ... -> HEAD)
	var rootToHead []*object.Commit
	for i := len(commits) - 1; i >= 0; i-- {
		rootToHead = append(rootToHead, commits[i])
	}

	// 核心修改逻辑：重建历史链条
	storer := repo.Storer
	var currentParentHash plumbing.Hash

	for i, oldCommit := range rootToHead {
		oldTree, err := oldCommit.Tree()
		if err != nil {
			return fmt.Errorf("get tree for commit %s: %w", oldCommit.Hash.String(), err)
		}

		var parentHashes []plumbing.Hash
		if i > 0 { // 非根提交
			parentHashes = []plumbing.Hash{currentParentHash}
		}

		message := oldCommit.Message
		author := oldCommit.Author
		//when := oldCommit.Author.When

		// 检查是否是目标 commit，如果是，则修改 message，更新 Committer 时间
		if oldCommit.Hash == targetHash {
			message = newCommitMsg
			// 注意：为了保持 git rebase 的惯例，我们保留原作者信息 (Author)，
			// 但更新提交者信息 (Committer) 和时间。
		}

		// 创建新的 commit 对象
		newCommit := &object.Commit{
			Author:       author,
			Committer:    object.Signature{Name: UserName, Email: UserEmail, When: time.Now()}, // 使用新的 Committer 和时间
			Message:      message,
			TreeHash:     oldTree.Hash,
			ParentHashes: parentHashes,
		}

		obj := storer.NewEncodedObject()
		if err := newCommit.Encode(obj); err != nil {
			return fmt.Errorf("encode rebased commit: %w", err)
		}
		currentParentHash, err = storer.SetEncodedObject(obj)
		if err != nil {
			return fmt.Errorf("store rebased commit: %w", err)
		}
	}

	// 设置新的引用
	finalHeadHash := currentParentHash
	mainRef := plumbing.NewHashReference(refName, finalHeadHash)
	if err := repo.Storer.SetReference(mainRef); err != nil {
		return fmt.Errorf("set ref: %w", err)
	}

	// 强制推送
	err = repo.Push(&git.PushOptions{
		Auth:  auth,
		Force: true,
		RefSpecs: []ggconfig.RefSpec{
			ggconfig.RefSpec(fmt.Sprintf("%s:%s", refName, refName)),
		},
		Progress: io.Discard,
	})
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}

	fmt.Printf("成功修改 commit %s 的信息，并重写历史\n", commitHash)
	return nil
}

// gomobile bind -o mixgram.aar -target="android/arm,android/arm64" -androidapi 21 -javapkg="com.donut.mixgram" -ldflags="-w -s" ./core
