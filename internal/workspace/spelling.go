package workspace

import "strings"

// Conservative common-error phrases. Fictional names and unrestricted single
// characters are deliberately excluded; every match remains a suggestion.
var spellingPairs = [][2]string{
	{"迫不急待", "迫不及待"}, {"再接再励", "再接再厉"},
	{"一愁莫展", "一筹莫展"}, {"不径而走", "不胫而走"},
	{"相形见拙", "相形见绌"}, {"走头无路", "走投无路"},
	{"随声附合", "随声附和"},
	{"滥芋充数", "滥竽充数"}, {"谈笑风声", "谈笑风生"},
	{"甘败下风", "甘拜下风"}, {"穿流不息", "川流不息"},
	{"出奇不意", "出其不意"}, {"一如继往", "一如既往"},
	{"按步就班", "按部就班"}, {"默守成规", "墨守成规"},
	{"一股作气", "一鼓作气"},
	{"声名雀起", "声名鹊起"}, {"无精打彩", "无精打采"},

	{"人情事故", "人情世故"}, {"变本加利", "变本加厉"},
	{"不记其数", "不计其数"}, {"莫不关心", "漠不关心"},
	{"大义禀然", "大义凛然"},

	{"悬梁刺骨", "悬梁刺股"}, {"原形必露", "原形毕露"},
	{"消声匿迹", "销声匿迹"},
	{"振耳欲聋", "震耳欲聋"},
	{"眼花瞭乱", "眼花缭乱"}, {"脍灸人口", "脍炙人口"},
	{"娇揉造作", "矫揉造作"},
	{"提心掉胆", "提心吊胆"}, {"世外桃园", "世外桃源"},
}

func spellingIssues(paragraph string, number int) []Issue {
	out := []Issue{}
	for _, pair := range spellingPairs {
		if pair[0] == pair[1] {
			continue
		}
		if strings.Contains(paragraph, pair[0]) {
			out = append(out, Issue{Kind: "疑似错别字", Paragraph: number, Quote: pair[0], Message: "建议将“" + pair[0] + "”改为“" + pair[1] + "”，请结合语境确认。"})
		}
	}
	return out
}
