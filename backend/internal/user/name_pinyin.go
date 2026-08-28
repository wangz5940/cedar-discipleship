package user

import (
	"strconv"
	"strings"
	"unicode"
)

func memberNamePinyin(name string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(name) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(unicode.ToLower(r))
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case unicode.Is(unicode.Han, r):
			if value := commonNamePinyin[r]; value != "" {
				b.WriteString(value)
			} else {
				b.WriteString("u")
				b.WriteString(strconv.FormatInt(int64(r), 16))
			}
		}
	}
	return normalizeUsername(b.String())
}

var commonNamePinyin = map[rune]string{
	'一': "yi", '丁': "ding", '七': "qi", '万': "wan", '三': "san", '上': "shang", '下': "xia", '世': "shi",
	'东': "dong", '中': "zhong", '丹': "dan", '丽': "li", '义': "yi", '之': "zhi", '乐': "le", '乔': "qiao",
	'书': "shu", '云': "yun", '亚': "ya", '亮': "liang", '仁': "ren", '仕': "shi", '令': "ling", '以': "yi",
	'伟': "wei", '伦': "lun", '伯': "bo", '佳': "jia", '俊': "jun", '保': "bao", '信': "xin", '倩': "qian",
	'健': "jian", '元': "yuan", '光': "guang", '兰': "lan", '兴': "xing", '兵': "bing", '军': "jun", '冠': "guan",
	'冰': "bing", '凡': "fan", '凤': "feng", '凯': "kai", '刚': "gang", '利': "li", '剑': "jian", '勇': "yong",
	'华': "hua", '博': "bo", '卫': "wei", '双': "shuang", '发': "fa", '君': "jun", '启': "qi", '和': "he",
	'哲': "zhe", '嘉': "jia", '国': "guo", '圣': "sheng", '坤': "kun", '培': "pei", '士': "shi", '夏': "xia",
	'天': "tian", '奇': "qi", '奕': "yi", '如': "ru", '娜': "na", '子': "zi", '宁': "ning", '宇': "yu",
	'安': "an", '宏': "hong", '宗': "zong", '宜': "yi", '宝': "bao", '家': "jia", '宸': "chen", '富': "fu",
	'小': "xiao", '少': "shao", '山': "shan", '岩': "yan", '峰': "feng", '川': "chuan", '帅': "shuai", '希': "xi",
	'平': "ping", '庆': "qing", '康': "kang", '建': "jian", '强': "qiang", '彦': "yan", '彬': "bin", '彤': "tong",
	'彭': "peng", '影': "ying", '德': "de", '心': "xin", '志': "zhi", '忠': "zhong", '怡': "yi", '恩': "en",
	'悦': "yue", '惠': "hui", '慧': "hui", '成': "cheng", '才': "cai", '文': "wen", '斌': "bin", '新': "xin",
	'方': "fang", '旭': "xu", '明': "ming", '星': "xing", '春': "chun", '晓': "xiao", '晨': "chen", '景': "jing",
	'晶': "jing", '智': "zhi", '月': "yue", '有': "you", '朋': "peng", '朗': "lang", '木': "mu", '本': "ben",
	'杰': "jie", '松': "song", '林': "lin", '柏': "bai", '栋': "dong", '桂': "gui", '梁': "liang", '梅': "mei",
	'梓': "zi", '梦': "meng", '森': "sen", '楠': "nan", '欣': "xin", '正': "zheng", '武': "wu", '毅': "yi",
	'民': "min", '永': "yong", '江': "jiang", '沐': "mu", '沛': "pei", '河': "he", '波': "bo", '泽': "ze",
	'洋': "yang", '洪': "hong", '浩': "hao", '海': "hai", '涛': "tao", '润': "run", '涵': "han", '淑': "shu",
	'清': "qing", '源': "yuan", '滢': "ying", '洁': "jie", '炜': "wei", '烨': "ye", '焕': "huan", '然': "ran",
	'燕': "yan", '玉': "yu", '玥': "yue", '玲': "ling", '珊': "shan", '珍': "zhen", '琪': "qi", '琳': "lin",
	'瑶': "yao", '瑞': "rui", '生': "sheng", '田': "tian", '男': "nan", '畅': "chang", '白': "bai", '益': "yi",
	'真': "zhen", '磊': "lei", '祥': "xiang", '福': "fu", '秀': "xiu", '秋': "qiu", '立': "li", '章': "zhang",
	'笑': "xiao", '红': "hong", '绍': "shao", '维': "wei", '美': "mei", '翔': "xiang", '翠': "cui", '耀': "yao",
	'聪': "cong", '胜': "sheng", '腾': "teng", '良': "liang", '艳': "yan", '艺': "yi", '芳': "fang", '若': "ruo",
	'英': "ying", '茜': "qian", '荣': "rong", '莉': "li", '莹': "ying", '菁': "jing", '菲': "fei", '萍': "ping",
	'蒙': "meng", '蓉': "rong", '薇': "wei", '虎': "hu", '虹': "hong", '裕': "yu", '豪': "hao", '贝': "bei",
	'贤': "xian", '超': "chao", '越': "yue", '轩': "xuan", '辉': "hui", '辰': "chen", '达': "da", '远': "yuan",
	'迪': "di", '道': "dao", '邦': "bang", '金': "jin", '鑫': "xin", '钰': "yu", '铭': "ming", '锋': "feng",
	'锦': "jin", '阳': "yang", '雅': "ya", '雨': "yu", '雪': "xue", '雯': "wen", '霖': "lin", '青': "qing",
	'静': "jing", '韦': "wei", '颖': "ying", '飞': "fei", '鹏': "peng", '龙': "long",
	'赵': "zhao", '钱': "qian", '孙': "sun", '李': "li", '周': "zhou", '吴': "wu", '郑': "zheng", '王': "wang",
	'冯': "feng", '陈': "chen", '褚': "chu", '蒋': "jiang", '沈': "shen", '韩': "han", '杨': "yang",
	'朱': "zhu", '秦': "qin", '尤': "you", '许': "xu", '何': "he", '吕': "lv", '施': "shi", '张': "zhang",
	'孔': "kong", '曹': "cao", '严': "yan", '魏': "wei", '陶': "tao", '姜': "jiang",
	'戚': "qi", '谢': "xie", '邹': "zou", '喻': "yu", '水': "shui", '窦': "dou",
	'苏': "su", '潘': "pan", '葛': "ge", '奚': "xi", '范': "fan", '郎': "lang", '鲁': "lu",
	'昌': "chang", '马': "ma", '苗': "miao", '花': "hua", '俞': "yu",
	'任': "ren", '袁': "yuan", '柳': "liu", '鲍': "bao", '史': "shi", '唐': "tang", '费': "fei", '廉': "lian",
	'岑': "cen", '薛': "xue", '雷': "lei", '贺': "he", '倪': "ni", '汤': "tang", '滕': "teng", '殷': "yin",
	'罗': "luo", '毕': "bi", '郝': "hao", '邬': "wu", '常': "chang", '于': "yu",
	'时': "shi", '傅': "fu", '皮': "pi", '卞': "bian", '齐': "qi", '顾': "gu", '孟': "meng",
	'黄': "huang", '穆': "mu", '萧': "xiao", '尹': "yin",
}
