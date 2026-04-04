package agents

import (
	"regexp"
	"strings"
)

// preProcessQuotes 使用正则表达式预处理 JSON 字符串
// 只保留特定位置的引号：{\s*"，"\s*:，:\s*"，"\s*}，"\s*,
// 其他位置的引号替换为空格
func preProcessQuotes(content string) string {
	// 使用占位符替换需要保留的引号
	placeholder := "\x00QUOTE\x00"

	// 模式 1: {\s*" - { 后面的引号（key 的开始）
	re1 := regexp.MustCompile(`(\{\s*)"`)
	content = re1.ReplaceAllString(content, "$1"+placeholder)

	// 模式 2: "\s*: - 引号后面跟冒号（key 的结束）
	// 注意：需要排除已经被模式 1 保护的引号
	re2 := regexp.MustCompile(`"(\s*:)`)
	content = re2.ReplaceAllString(content, placeholder+"$1")

	// 模式 3: :\s*" - 冒号后面的引号（value 的开始，如果是字符串 value）
	re3 := regexp.MustCompile(`(:\s*)"`)
	content = re3.ReplaceAllString(content, "$1"+placeholder)

	// 模式 4: "\s*\} - 引号后面跟}（value 的结束）
	re4 := regexp.MustCompile(`"(\s*\})`)
	content = re4.ReplaceAllString(content, placeholder+"$1")

	// 模式 5: "\s*, - 引号后面跟逗号（key/value 的结束）
	re5 := regexp.MustCompile(`"(\s*,)`)
	content = re5.ReplaceAllString(content, placeholder+"$1")

	// 模式 6: ,\s*" - , 后面的引号（key-value 对后的下一个 key 开始）
	re6 := regexp.MustCompile(`(,\s*)"`)
	content = re6.ReplaceAllString(content, "$1"+placeholder)

	// 模式 7: [\s*" - , 后面的引号（key-value 对后的下一个 key 开始）
	re7 := regexp.MustCompile(`(\[\s*)"`)
	content = re7.ReplaceAllString(content, "$1"+placeholder)

	// 模式 8: "\s*] - , 后面的引号（key-value 对后的下一个 key 开始）
	re8 := regexp.MustCompile(`"(\s*\])`)
	content = re8.ReplaceAllString(content, placeholder+"$1")

	// 将所有剩余的引号替换为空格
	content = strings.ReplaceAll(content, `"`, " ")

	// 恢复被标记的引号
	content = strings.ReplaceAll(content, placeholder, `"`)

	return content
}
