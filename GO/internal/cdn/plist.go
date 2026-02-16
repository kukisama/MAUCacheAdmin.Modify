package cdn

import (
	"encoding/xml"
	"strings"
)

// Plist XML 解析
// 对应 PowerShell ConvertFrom-Plist.ps1 的递归解析器
// MAU 的 Plist 只用到：dict、array、string、integer、true、false

// plistDoc 代表整个 plist XML 文档
type plistDoc struct {
	XMLName xml.Name     `xml:"plist"`
	Items   []plistValue `xml:",any"`
}

// plistValue 代表 plist 中的任意节点
type plistValue struct {
	XMLName xml.Name
	Content string       `xml:",chardata"`
	Items   []plistValue `xml:",any"`
}

// PackageList 存储从 AppID.xml 解析出的包信息
type PackageList struct {
	Locations []string // Location / BinaryUpdaterLocation / FullUpdaterLocation 字段值
	Versions  []string // Update Version 字段值
}

// AllURIs 返回去重后的所有下载 URI
func (p *PackageList) AllURIs() []string {
	seen := make(map[string]bool)
	var result []string
	for _, u := range p.Locations {
		if u != "" && !seen[u] {
			seen[u] = true
			result = append(result, u)
		}
	}
	return result
}

// ParsePlistPackages 解析 AppID.xml，提取包信息
// 输入是一个 Plist array of dict，每个 dict 包含 Location、Update Version 等 key
// 对应 PowerShell: ConvertFrom-Plist → ConvertFrom-AppPackageDictionary
func ParsePlistPackages(xmlStr string) (*PackageList, error) {
	var doc plistDoc
	if err := xml.Unmarshal([]byte(xmlStr), &doc); err != nil {
		return nil, err
	}

	result := &PackageList{}

	// root 应该包含一个 array，里面的每个 item 是 dict
	arrayNode := findChildInSlice(doc.Items, "array")
	if arrayNode == nil && len(doc.Items) > 0 {
		// 可能 root 的第一个子元素就是 array
		arrayNode = &doc.Items[0]
	}
	if arrayNode == nil {
		return result, nil
	}

	for _, dictNode := range arrayNode.Items {
		if dictNode.XMLName.Local != "dict" {
			continue
		}
		m := parseDictToMap(dictNode)
		// 收集所有下载 URL（Location、BinaryUpdaterLocation、FullUpdaterLocation）
		// 对应 Get-MAUCacheDownloadJobs.ps1 第 34 行的合并逻辑
		for _, key := range []string{"Location", "BinaryUpdaterLocation", "FullUpdaterLocation"} {
			if v, ok := m[key]; ok && v != "" {
				result.Locations = append(result.Locations, v)
			}
		}
		if v, ok := m["Update Version"]; ok {
			result.Versions = append(result.Versions, v)
		}
	}

	return result, nil
}

// ParsePlistVersion 从 chk.xml 中解析 Update Version
// 对应 Get-MAUApp.ps1 第 47-52 行
func ParsePlistVersion(xmlStr string) string {
	var doc plistDoc
	if err := xml.Unmarshal([]byte(xmlStr), &doc); err != nil {
		return "unknown"
	}

	dictNode := findChildInSlice(doc.Items, "dict")
	if dictNode == nil {
		return "unknown"
	}

	m := parseDictToMap(*dictNode)
	if v, ok := m["Update Version"]; ok {
		return v
	}
	return "unknown"
}

// ParsePlistStringArray 从 history.xml 中解析版本号列表
// 对应 Get-MAUApp.ps1 第 63 行: [string[]]$historicAppVersions
func ParsePlistStringArray(xmlStr string) []string {
	var doc plistDoc
	if err := xml.Unmarshal([]byte(xmlStr), &doc); err != nil {
		return nil
	}

	arrayNode := findChildInSlice(doc.Items, "array")
	if arrayNode == nil {
		return nil
	}

	var result []string
	for _, item := range arrayNode.Items {
		if item.XMLName.Local == "string" {
			result = append(result, strings.TrimSpace(item.Content))
		}
	}
	return result
}

// parseDictToMap 把 Plist dict 节点转成 Go map
// 对应 ConvertFrom-Plist.ps1 的 dict 分支：key-value 交替排列
func parseDictToMap(dict plistValue) map[string]string {
	m := make(map[string]string)
	items := dict.Items
	for i := 0; i < len(items)-1; i += 2 {
		if items[i].XMLName.Local == "key" {
			key := strings.TrimSpace(items[i].Content)
			val := items[i+1]
			switch val.XMLName.Local {
			case "string":
				m[key] = strings.TrimSpace(val.Content)
			case "integer":
				m[key] = strings.TrimSpace(val.Content)
			case "true":
				m[key] = "true"
			case "false":
				m[key] = "false"
			}
		}
	}
	return m
}

// findChildInSlice 在 plistValue 切片中查找指定名称的子节点
func findChildInSlice(items []plistValue, name string) *plistValue {
	for i := range items {
		if items[i].XMLName.Local == name {
			return &items[i]
		}
	}
	return nil
}
