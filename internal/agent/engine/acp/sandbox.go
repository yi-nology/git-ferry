package acp

import (
	"fmt"
	"github.com/oekit/pkg/errors"
	"os"
	"path/filepath"

	"github.com/oekit/pkg/pathx"
)

// errEmptyPath 路径为空。
var errEmptyPath = errors.New("path 不能为空")

// sandbox 是外部 agent 的**文件系统边界**:一个根目录 + 一套判定。
//
// ## 为什么要抽出来
//
// fs/* 与 terminal/* 是两条能力,边界判定必须是**同一份**:
// 各写一套 ⇒ 迟早漂移(本仓就漂了:terminal 的 cwd 是**直接用 agent 给的**,
// 而文档写的是「命令的工作目录固定在 root 内」—— 实现根本没管,
// agent 报一个根外的 cwd 就能把命令挪到沙箱外)。
//
// ## 判定口径
//
// 路径必须在根内,且**已解析符号链接**:
//
//   - 只做 filepath.Clean 的话,工作区里一个指向 ~/.ssh/id_rsa 的软链
//     路径看起来仍在根下,实际读的是根外的文件 ⇒ 沙箱形同虚设;
//   - macOS 上 /tmp → /private/tmp、/var → /private/var,
//     根目录本身就在软链下 ⇒ 根也要解析,否则合法路径会被误判成越界。
//
// 相对路径的判定用 oekit 的 **pathx.SanitizeRelStrict(严格档)**:
// 宽松档的 ".." 抵消是**纯文本语义,看不见符号链接**
// ("a/../b" 在宽松档得到 "b",在严格档直接拒)—— 沙箱宁拒不推断。
type sandbox struct {
	// root 允许访问的根目录(**已解析符号链接**的绝对路径)。
	root string
}

// newSandbox 构造沙箱(解析根目录的符号链接)。
func newSandbox(root string) (*sandbox, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, errors.Wrap(err, "解析工作目录失败")
	}
	// **根目录本身也要解析**:macOS 上 /tmp 是 /private/tmp 的软链,
	// 不解析就会把工作区里的合法文件误判成越界。
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, errors.Wrap(err, "解析工作目录符号链接失败")
	}
	return &sandbox{root: real}, nil
}

// inside 校验路径在沙箱根内。
//
// 相对根目录(根自身)算「在根内」—— 它不是越界,只是不是文件
// (调用方按「是目录」处理)。
func (s *sandbox) inside(path string) error {
	rel, err := filepath.Rel(s.root, path)
	if err != nil {
		return fmt.Errorf("路径越界: %s", path)
	}
	if rel == "." {
		return nil
	}
	if _, ok := pathx.SanitizeRelStrict(rel); !ok {
		return fmt.Errorf("路径越界(只允许访问 %s 内的文件): %s", s.root, path)
	}
	return nil
}

// resolve 解析并校验路径:必须在沙箱根内(**含符号链接**)。
//
// 目标**不存在**时(写新文件、或读一个不存在的文件)⇒
// 解析**父目录**的符号链接后再拼文件名 ——
// 包含关系仍要在**解析后**判,否则根目录本身在软链下(macOS 的 /var)会误判。
func (s *sandbox) resolve(path string) (string, error) {
	if path == "" {
		return "", errEmptyPath
	}
	if !filepath.IsAbs(path) {
		// **相对路径:先过严格档,再拼根目录**。
		// filepath.Join 会把 "a/../b" 在**文本上**抵消掉,而抵消是纯文本语义、
		// 看不见符号链接("a" 若是软链,真实落点可能落在根外)⇒ 抵消之前就拒。
		if _, ok := pathx.SanitizeRelStrict(path); !ok {
			return "", fmt.Errorf("路径越界(只允许访问 %s 内的文件): %s", s.root, path)
		}
		path = filepath.Join(s.root, path)
	}
	clean := filepath.Clean(path)
	real, err := filepath.EvalSymlinks(clean)
	if err != nil {
		if os.IsNotExist(err) {
			// 目标还不存在(写新文件):解析父目录,再拼文件名。
			parent, perr := filepath.EvalSymlinks(filepath.Dir(clean))
			if perr != nil {
				return "", errors.Wrap(perr, "解析父目录失败")
			}
			joined := filepath.Join(parent, filepath.Base(clean))
			if err := s.inside(joined); err != nil {
				return "", err
			}
			return joined, nil
		}
		return "", errors.Wrap(err, "解析路径失败")
	}
	// **在解析之后判包含**:macOS 上 /var → /private/var,
	// 先用未解析的路径比会误判合法路径越界。
	if err := s.inside(real); err != nil {
		return "", err
	}
	return real, nil
}

// resolveCwd 解析 agent 指定的工作目录:必须在沙箱根内。
//
// **不存在 ⇒ 直接拒**:工作目录要立刻被 exec 用,不存在时没有「写新文件」
// 那条回退路径可用。
func (s *sandbox) resolveCwd(cwd string) (string, error) {
	if cwd == "" {
		return s.root, nil
	}
	abs := cwd
	if !filepath.IsAbs(abs) {
		// 同 resolve:相对路径先过严格档(显式 ".." 一律拒)。
		if _, ok := pathx.SanitizeRelStrict(abs); !ok {
			return "", fmt.Errorf("路径越界(只允许访问 %s 内的文件): %s", s.root, cwd)
		}
		abs = filepath.Join(s.root, abs)
	}
	real, err := filepath.EvalSymlinks(filepath.Clean(abs))
	if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("工作目录不可用(%s)", cwd))
	}
	if err := s.inside(real); err != nil {
		return "", err
	}
	return real, nil
}
