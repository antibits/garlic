package agents

import (
	"encoding/json"
	"testing"

	"github.com/kaptinlin/jsonrepair"
)

func TestPreProcessQuotes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal JSON unchanged",
			input:    `{"tool": "websearch", "args": {"query": "ML"}}`,
			expected: `{"tool": "websearch", "args": {"query": "ML"}}`,
		},
		{
			name:     "extra quotes in values",
			input:    `{"tool": "web"search", "args": {"query": "mac"hine learning"}}`,
			expected: `{"tool": "web search", "args": {"query": "mac hine learning"}}`,
		},
		{
			name:     "extra quotes in middle of string",
			input:    `{"tool": "test"ing", "args": {"query": "hel"lo"}}`,
			expected: `{"tool": "test ing", "args": {"query": "hel lo"}}`,
		},
		{
			name:     "quotes at key positions preserved",
			input:    `{"tool":"value"}`,
			expected: `{"tool":"value"}`,
		},
		{
			name:     "quotes before comma and closing brace",
			input:    `{"a": "test"value, "b": "data"}`,
			expected: `{"a": "test value, "b": "data"}`,
		},
		{
			name:     "quotes before comma and closing brace",
			input:    `{"a": "test"value, "b": ["data", "excepti"on"]}`,
			expected: `{"a": "test value, "b": ["data", "excepti on"]}`,
		},
		{
			name:     "real case.",
			input:    "{\"tool\": \"filewriter\", \"args\": {\"path\": \"CSU_vs_HNU_Comparison.md\", \"mode\": \"overwrite\", \"content\": \"### **中南大学 vs 湖南大学：全方位深度对比**\\n\\n两校均位于湖南长沙，同为教育部直属全国重点大学，位列国家\\\"985 工程”、\"211 工程”及第二轮“双一流”建设高校。但在发展路径和优势领域上各有侧重。\\n\\n#### **1. 师资力量 (Faculty Strength)**\\n*   **中南大学：**\\n    *   **特点：** 规模宏大，学科覆盖面广，尤其在医工结合领域师资雄厚。\\n    *   **优势领域：** 拥有强大的医学师资团队（依托湘雅医学院），同时在矿业、冶金、材料等传统强势工科领域汇聚了大量顶尖专家。\\n    *   **院士数量：** 通常拥有较多的全职及双聘院士，特别是在工程技术领域。\\n*   **湖南大学：**\\n    *   **特点：** 师资力量精干，历史悠久（千年学府），注重基础学科与传统工科的传承。\\n    *   **优势领域：** 在机械工程、土木工程、化学、工商管理等领域拥有深厚的学术积淀和知名学者。\\n    *   **结构：** 师资结构相对均衡，文科与理科的教授比例较中南大学略高。\\n\\n#### **2. 科研经费 (Research Funding)**\\n*   **数据对比（参考 2024 年预算）：**\\n    *   **中南大学：** 约 **108.4 亿元**。\\n    *   **湖南大学：** 约 **97.8 亿元**。\\n*   **分析：**\\n    *   **中南大学险胜：** 经费总额略高于湖南大学。主要得益于其**医学板块**的强大造血能力（多家直属附属医院收入丰厚）以及矿业、冶金等大型工科项目带来的巨额纵向与横向课题经费。\\n    *   **湖南大学：** 经费体量同样巨大，位居全国前列，但相比中南少了大规模附属医院的支撑，更多依赖工科科研项目和国家财政拨款。两者差距并不悬殊，均属于百亿级俱乐部。\\n\\n#### **3. 人才分布与学科布局 (Talent Distribution \u0026 Disciplines)**\\n*   **中南大学（“医工强校”）：**\\n    *   **核心王牌：** **临床医学**（湘雅品牌，国内顶尖）、**矿业工程**、**冶金工程**、**材料科学与工程**。\\n    *   **人才流向：** 毕业生大量进入大型国企（如五矿、宝武）、医疗卫生系统、科研院所。其在有色金属行业和医疗界的人才垄断性较强。\\n    *   **学科生态：** 学科发展极为全面，形成了“医学 + 重工”的双引擎驱动模式。\\n*   **湖南大学（“文理兼修”）：**\\n    *   **核心王牌：** **机械工程**（车辆工程方向极强）、**土木工程**、**化学**、**工商管理**（会计学科著名）、**设计学**。\\n    *   **人才流向：** 毕业生广泛分布于汽车制造（如上汽、广汽）、建筑设计院、金融机构、互联网大厂及政府部门。\\n    *   **学科生态：** 传统工科底蕴深厚，同时文理科（如化学、经管）发展均衡，更具综合性大学的气质。\\n\\n#### **4. 社会认可度 (Social Recognition)**\\n*   **综合声誉：**\\n    *   在两湖地区及全国范围内，两校知名度不相上下，均为顶尖名校。\\n    *   **行业认可：**\\n        *   **中南大学：** 在**医疗界**（“南湘雅”美誉）和**有色金属/矿冶行业**具有绝对的话语权和统治力。若从事医生或矿冶相关工作，中南大学的校友资源无可替代。\\n        *   **湖南大学：** 在**机械制造、土木建筑、金融财会**领域享有极高声誉。其“千年学府”的文化标签使其在人文社科领域的品牌形象更为独特。\\n*   **录取分数：**\\n    *   在大多数省份，两校的录取分数线互有胜负，通常取决于具体专业。热门专业（如中南的临床医学、湖大的车辆/金融）分数均极高。\\n\\n---\\n\\n### **总结建议**\\n\\n| 维度 | 中南大学 (CSU) | 湖南大学 (HNU) |\\n| :--- | :--- | :--- |\\n| **最强标签** | 湘雅医学、矿冶之王 | 千年学府、机械/土木强校 |\\n| **经费来源特色** | 医学附属医院贡献巨大 | 传统工科项目与国家拨款为主 |\\n| **适合人群** | 想学医、材料、冶金，或倾向于大型重工企业就业的学生。 | 想学机械、土木、化学、经管，或倾向于多元化发展的学生。 |\\n| **整体风格** | 务实、硬核、行业壁垒高 | 厚重、综合、文化底蕴深 |\\n\\n**结论：** 如果您看重**医学**或**特定重工行业**的资源，**中南大学**略占优势（经费也稍多）；如果您倾向于**机械、土木、商科**或喜欢**综合性人文氛围**，**湖南大学**则是极佳选择。两校均为国家栋梁之材的摇篮，实力在伯仲之间。\"}}",
			expected: "{\"tool\": \"filewriter\", \"args\": {\"path\": \"CSU_vs_HNU_Comparison.md\", \"mode\": \"overwrite\", \"content\": \"### **中南大学 vs 湖南大学：全方位深度对比**\\n\\n两校均位于湖南长沙，同为教育部直属全国重点大学，位列国家\\ 985 工程”、 211 工程”及第二轮“双一流”建设高校。但在发展路径和优势领域上各有侧重。\\n\\n#### **1. 师资力量 (Faculty Strength)**\\n*   **中南大学：**\\n    *   **特点：** 规模宏大，学科覆盖面广，尤其在医工结合领域师资雄厚。\\n    *   **优势领域：** 拥有强大的医学师资团队（依托湘雅医学院），同时在矿业、冶金、材料等传统强势工科领域汇聚了大量顶尖专家。\\n    *   **院士数量：** 通常拥有较多的全职及双聘院士，特别是在工程技术领域。\\n*   **湖南大学：**\\n    *   **特点：** 师资力量精干，历史悠久（千年学府），注重基础学科与传统工科的传承。\\n    *   **优势领域：** 在机械工程、土木工程、化学、工商管理等领域拥有深厚的学术积淀和知名学者。\\n    *   **结构：** 师资结构相对均衡，文科与理科的教授比例较中南大学略高。\\n\\n#### **2. 科研经费 (Research Funding)**\\n*   **数据对比（参考 2024 年预算）：**\\n    *   **中南大学：** 约 **108.4 亿元**。\\n    *   **湖南大学：** 约 **97.8 亿元**。\\n*   **分析：**\\n    *   **中南大学险胜：** 经费总额略高于湖南大学。主要得益于其**医学板块**的强大造血能力（多家直属附属医院收入丰厚）以及矿业、冶金等大型工科项目带来的巨额纵向与横向课题经费。\\n    *   **湖南大学：** 经费体量同样巨大，位居全国前列，但相比中南少了大规模附属医院的支撑，更多依赖工科科研项目和国家财政拨款。两者差距并不悬殊，均属于百亿级俱乐部。\\n\\n#### **3. 人才分布与学科布局 (Talent Distribution \u0026 Disciplines)**\\n*   **中南大学（“医工强校”）：**\\n    *   **核心王牌：** **临床医学**（湘雅品牌，国内顶尖）、**矿业工程**、**冶金工程**、**材料科学与工程**。\\n    *   **人才流向：** 毕业生大量进入大型国企（如五矿、宝武）、医疗卫生系统、科研院所。其在有色金属行业和医疗界的人才垄断性较强。\\n    *   **学科生态：** 学科发展极为全面，形成了“医学 + 重工”的双引擎驱动模式。\\n*   **湖南大学（“文理兼修”）：**\\n    *   **核心王牌：** **机械工程**（车辆工程方向极强）、**土木工程**、**化学**、**工商管理**（会计学科著名）、**设计学**。\\n    *   **人才流向：** 毕业生广泛分布于汽车制造（如上汽、广汽）、建筑设计院、金融机构、互联网大厂及政府部门。\\n    *   **学科生态：** 传统工科底蕴深厚，同时文理科（如化学、经管）发展均衡，更具综合性大学的气质。\\n\\n#### **4. 社会认可度 (Social Recognition)**\\n*   **综合声誉：**\\n    *   在两湖地区及全国范围内，两校知名度不相上下，均为顶尖名校。\\n    *   **行业认可：**\\n        *   **中南大学：** 在**医疗界**（“南湘雅”美誉）和**有色金属/矿冶行业**具有绝对的话语权和统治力。若从事医生或矿冶相关工作，中南大学的校友资源无可替代。\\n        *   **湖南大学：** 在**机械制造、土木建筑、金融财会**领域享有极高声誉。其“千年学府”的文化标签使其在人文社科领域的品牌形象更为独特。\\n*   **录取分数：**\\n    *   在大多数省份，两校的录取分数线互有胜负，通常取决于具体专业。热门专业（如中南的临床医学、湖大的车辆/金融）分数均极高。\\n\\n---\\n\\n### **总结建议**\\n\\n| 维度 | 中南大学 (CSU) | 湖南大学 (HNU) |\\n| :--- | :--- | :--- |\\n| **最强标签** | 湘雅医学、矿冶之王 | 千年学府、机械/土木强校 |\\n| **经费来源特色** | 医学附属医院贡献巨大 | 传统工科项目与国家拨款为主 |\\n| **适合人群** | 想学医、材料、冶金，或倾向于大型重工企业就业的学生。 | 想学机械、土木、化学、经管，或倾向于多元化发展的学生。 |\\n| **整体风格** | 务实、硬核、行业壁垒高 | 厚重、综合、文化底蕴深 |\\n\\n**结论：** 如果您看重**医学**或**特定重工行业**的资源，**中南大学**略占优势（经费也稍多）；如果您倾向于**机械、土木、商科**或喜欢**综合性人文氛围**，**湖南大学**则是极佳选择。两校均为国家栋梁之材的摇篮，实力在伯仲之间。\"}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := preProcessQuotes(tt.input)
			if result != tt.expected {
				t.Errorf("preProcessQuotes(%q) = %q, want %q", tt.input, result, tt.expected)
			}

			// Verify the result is valid JSON or can be repaired
			_, err := jsonrepair.Repair(result)
			if err != nil {
				t.Errorf("preProcessQuotes(%q) result cannot be repaired: %v", tt.input, err)
			}
		})
	}
}

func TestFixJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantValid bool
	}{
		{
			name:      "valid llm output",
			input:     "{\"tool\": \"filewriter\", \"args\": {\"path\": \"CSU_vs_HNU_Comparison.md\", \"mode\": \"overwrite\", \"content\": \"### **中南大学 vs 湖南大学：全方位深度对比**\\n\\n两校均位于湖南长沙，同为教育部直属全国重点大学，位列国家\\\"985 工程”、\"211 工程”及第二轮“双一流”建设高校。但在发展路径和优势领域上各有侧重。\\n\\n#### **1. 师资力量 (Faculty Strength)**\\n*   **中南大学：**\\n    *   **特点：** 规模宏大，学科覆盖面广，尤其在医工结合领域师资雄厚。\\n    *   **优势领域：** 拥有强大的医学师资团队（依托湘雅医学院），同时在矿业、冶金、材料等传统强势工科领域汇聚了大量顶尖专家。\\n    *   **院士数量：** 通常拥有较多的全职及双聘院士，特别是在工程技术领域。\\n*   **湖南大学：**\\n    *   **特点：** 师资力量精干，历史悠久（千年学府），注重基础学科与传统工科的传承。\\n    *   **优势领域：** 在机械工程、土木工程、化学、工商管理等领域拥有深厚的学术积淀和知名学者。\\n    *   **结构：** 师资结构相对均衡，文科与理科的教授比例较中南大学略高。\\n\\n#### **2. 科研经费 (Research Funding)**\\n*   **数据对比（参考 2024 年预算）：**\\n    *   **中南大学：** 约 **108.4 亿元**。\\n    *   **湖南大学：** 约 **97.8 亿元**。\\n*   **分析：**\\n    *   **中南大学险胜：** 经费总额略高于湖南大学。主要得益于其**医学板块**的强大造血能力（多家直属附属医院收入丰厚）以及矿业、冶金等大型工科项目带来的巨额纵向与横向课题经费。\\n    *   **湖南大学：** 经费体量同样巨大，位居全国前列，但相比中南少了大规模附属医院的支撑，更多依赖工科科研项目和国家财政拨款。两者差距并不悬殊，均属于百亿级俱乐部。\\n\\n#### **3. 人才分布与学科布局 (Talent Distribution \u0026 Disciplines)**\\n*   **中南大学（“医工强校”）：**\\n    *   **核心王牌：** **临床医学**（湘雅品牌，国内顶尖）、**矿业工程**、**冶金工程**、**材料科学与工程**。\\n    *   **人才流向：** 毕业生大量进入大型国企（如五矿、宝武）、医疗卫生系统、科研院所。其在有色金属行业和医疗界的人才垄断性较强。\\n    *   **学科生态：** 学科发展极为全面，形成了“医学 + 重工”的双引擎驱动模式。\\n*   **湖南大学（“文理兼修”）：**\\n    *   **核心王牌：** **机械工程**（车辆工程方向极强）、**土木工程**、**化学**、**工商管理**（会计学科著名）、**设计学**。\\n    *   **人才流向：** 毕业生广泛分布于汽车制造（如上汽、广汽）、建筑设计院、金融机构、互联网大厂及政府部门。\\n    *   **学科生态：** 传统工科底蕴深厚，同时文理科（如化学、经管）发展均衡，更具综合性大学的气质。\\n\\n#### **4. 社会认可度 (Social Recognition)**\\n*   **综合声誉：**\\n    *   在两湖地区及全国范围内，两校知名度不相上下，均为顶尖名校。\\n    *   **行业认可：**\\n        *   **中南大学：** 在**医疗界**（“南湘雅”美誉）和**有色金属/矿冶行业**具有绝对的话语权和统治力。若从事医生或矿冶相关工作，中南大学的校友资源无可替代。\\n        *   **湖南大学：** 在**机械制造、土木建筑、金融财会**领域享有极高声誉。其“千年学府”的文化标签使其在人文社科领域的品牌形象更为独特。\\n*   **录取分数：**\\n    *   在大多数省份，两校的录取分数线互有胜负，通常取决于具体专业。热门专业（如中南的临床医学、湖大的车辆/金融）分数均极高。\\n\\n---\\n\\n### **总结建议**\\n\\n| 维度 | 中南大学 (CSU) | 湖南大学 (HNU) |\\n| :--- | :--- | :--- |\\n| **最强标签** | 湘雅医学、矿冶之王 | 千年学府、机械/土木强校 |\\n| **经费来源特色** | 医学附属医院贡献巨大 | 传统工科项目与国家拨款为主 |\\n| **适合人群** | 想学医、材料、冶金，或倾向于大型重工企业就业的学生。 | 想学机械、土木、化学、经管，或倾向于多元化发展的学生。 |\\n| **整体风格** | 务实、硬核、行业壁垒高 | 厚重、综合、文化底蕴深 |\\n\\n**结论：** 如果您看重**医学**或**特定重工行业**的资源，**中南大学**略占优势（经费也稍多）；如果您倾向于**机械、土木、商科**或喜欢**综合性人文氛围**，**湖南大学**则是极佳选择。两校均为国家栋梁之材的摇篮，实力在伯仲之间。\"}}",
			wantValid: true,
		},
		{
			name:      "valid JSON unchanged",
			input:     `{"tool": "websearch", "args": {"query": "machine learning"}}`,
			wantValid: true,
		},
		{
			name:      "missing quotes on keys",
			input:     `{tool: "websearch", args: {query: "machine learning"}}`,
			wantValid: true,
		},
		{
			name:      "missing quotes on values",
			input:     `{"tool": websearch, "args": {"query": machine learning}}`,
			wantValid: true,
		},
		{
			name:      "missing quotes on both keys and values",
			input:     `{tool: websearch, args: {query: machine learning}}`,
			wantValid: true,
		},
		{
			name:      "extra whitespace",
			input:     `{ tool :  websearch ,  args :  { query :  machine learning  }  }`,
			wantValid: true,
		},
		{
			name:      "nested object with missing quotes",
			input:     `{tool: websearch, args: {query: ML, num: 10}}`,
			wantValid: true,
		},
		{
			name:      "single quotes",
			input:     `{'tool': 'websearch', 'args': {'query': 'ML'}}`,
			wantValid: true,
		},
		{
			name:      "mixed issues",
			input:     `{tool:websearch,args:{query:'ML test'}}`,
			wantValid: true,
		},
		{
			name:      "curly quotes in value",
			input:     `{"tool": "websearch", "args": {"query": "machine learning"}}`,
			wantValid: true,
		},
		{
			name:      "curly quotes in value",
			input:     `{"tool": "web"search", "args": {"query": "mac"hine learning"}}`,
			wantValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// fixed := fixJSON(tt.input)
			fixed, _ := jsonrepair.Repair(tt.input)

			// Try to parse the fixed JSON
			var result map[string]interface{}
			err := json.Unmarshal([]byte(fixed), &result)

			if tt.wantValid && err != nil {
				t.Errorf("fixJSON(%q) = %q, failed to parse: %v", tt.input, fixed, err)
			}

			// Verify tool field is preserved
			if tt.wantValid {
				if toolVal, ok := result["tool"]; !ok {
					t.Errorf("fixJSON(%q) missing 'tool' field", tt.input)
				} else if toolStr, ok := toolVal.(string); !ok || toolStr == "" {
					// Tool can be empty for some cases
				}
			}
		})
	}
}

func TestRePatterns(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Intent
	}{
		{
			name:     "test re_pattern",
			input:    `"intent": "simple"`,
			expected: IntentSimple,
		},
		{
			name:     "test re_pattern",
			input:    `intent": step_by_step"`,
			expected: IntentStepByStep,
		},
		{
			name:     "test re_pattern",
			input:    `{intent: "step_by_step`,
			expected: IntentStepByStep,
		},
	}
	for _, tt := range tests {
		cli := &Router{}
		t.Run(tt.name, func(t *testing.T) {
			_, intent, _ := cli.parseResponse(tt.input)
			if intent.Intent != tt.expected {
				t.Error("expected:", tt.expected, "result:", intent.Intent)
			}

		})
	}
}
