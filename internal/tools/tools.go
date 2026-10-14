package tools

import "time"

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

func All() []Tool {
	return []Tool{
		{Name: "maya_status", Description: "读取当前 Maya 会话：版本、场景文件、帧范围、渲染器、选择、摄影机和对象数量。", Parameters: obj(map[string]any{}, nil)},
		{Name: "maya_scene", Description: "新建、打开、保存或另存 Maya 场景。", Parameters: obj(map[string]any{
			"action": enum("new、open、save、save_as、query", "new", "open", "save", "save_as", "query"),
			"path":   str("打开或另存的文件路径"),
			"type":   enum("另存格式", "mayaAscii", "mayaBinary"),
		}, []string{"action"})},
		{Name: "maya_list", Description: "按节点类型列出对象，或只列出当前选择。", Parameters: obj(map[string]any{
			"type":     str("Maya 节点类型，例如 transform、mesh、light、camera、nParticle"),
			"selected": boolean("为 true 时只返回当前选择"),
			"limit":    num("最多返回的数量，默认 300"),
		}, nil)},
		{Name: "maya_select", Description: "选择对象，或清空选择。", Parameters: obj(map[string]any{
			"names": namesProp(),
			"add":   boolean("追加选择而不是替换"),
			"clear": boolean("清空选择"),
		}, nil)},
		{Name: "maya_primitive", Description: "创建多边形基本体。kind 可以是 cube、sphere、cylinder、plane、torus、cone、pipe、prism。", Parameters: obj(map[string]any{
			"kind": str("基本体类型"), "name": str("变换节点名"),
			"width": num("宽"), "height": num("高"), "depth": num("深"), "radius": num("半径"),
			"sectionRadius": num("圆环截面半径"), "thickness": num("管壁厚度"), "length": num("棱柱长度"), "sides": num("边数"),
			"subdivisionsX": num("细分 X"), "subdivisionsY": num("细分 Y"), "subdivisionsZ": num("细分 Z"),
			"translate": vec("位移"), "rotate": vec("旋转"), "scale": vec("缩放"),
		}, []string{"kind"})},
		{Name: "maya_transform", Description: "移动、旋转或缩放对象。relative 为 true 时在当前值上增量修改。", Parameters: obj(map[string]any{
			"names": namesProp(), "translate": vec("位移"), "rotate": vec("旋转"), "scale": vec("缩放"),
			"relative": boolean("相对变换"),
		}, nil)},
		{Name: "maya_edit", Description: "建模编辑：extrude、bevel、smooth、delete、duplicate、combine、separate、freeze、center_pivot、history、rename、parent。", Parameters: obj(map[string]any{
			"action":    str("编辑动作"),
			"names":     namesProp(),
			"distance":  num("挤出距离"),
			"fraction":  num("倒角比例"),
			"segments":  num("倒角段数"),
			"divisions": num("平滑细分"),
			"name":      str("重命名"),
			"parent":    str("父对象，world 表示放到世界"),
		}, []string{"action"})},
		{Name: "maya_material", Description: "创建并指定材质。默认 standardSurface，失败时回退 lambert。颜色是 0 到 1 的 RGB。", Parameters: obj(map[string]any{
			"name": str("材质名"), "type": str("standardSurface、lambert 或 aiStandardSurface"),
			"color": vec("RGB 0-1"), "roughness": num("高光粗糙度"), "assignTo": namesProp(),
		}, nil)},
		{Name: "maya_camera", Description: "创建摄影机。", Parameters: obj(map[string]any{
			"name": str("名称"), "translate": vec("位移"), "rotate": vec("旋转"), "focalLength": num("焦距"),
		}, nil)},
		{Name: "maya_fx", Description: "特效：nparticle 全向发射器、ncloth 布料、field 力场（gravity/air/turbulence/vortex/radial/uniform/drag）、nucleus 解算器参数。", Parameters: obj(map[string]any{
			"kind": str("nparticle、ncloth、field、nucleus"), "name": str("名称"), "mesh": str("布料网格"),
			"field": str("力场类型"), "position": vec("位置"), "rate": num("发射速率"), "magnitude": num("力场强度"),
			"affect": namesProp(), "gravity": num("nucleus 重力"), "gravityDirection": vec("重力方向"), "airDensity": num("空气密度"),
		}, []string{"kind"})},
		{Name: "maya_simulate", Description: "解算与缓存：range 设置帧范围，time 跳帧，evaluate 逐帧解算，bake 烘焙结果，playblast 拍屏，geometry_cache 几何缓存。", Parameters: obj(map[string]any{
			"action": str("range、time、evaluate、bake、playblast、geometry_cache"),
			"names":  namesProp(), "start": num("起始帧"), "end": num("结束帧"), "min": num("起始帧"), "max": num("结束帧"),
			"time": num("当前帧"), "step": num("步长"), "width": num("宽度"), "height": num("高度"), "path": str("输出路径"),
		}, []string{"action"})},
		{Name: "maya_light", Description: "创建灯光。kind：ambient、directional、point、spot、area、aiSkyDome、aiArea。", Parameters: obj(map[string]any{
			"kind": str("灯光类型"), "name": str("名称"), "intensity": num("强度"), "color": vec("RGB 0-1"),
			"translate": vec("位移"), "rotate": vec("旋转"), "coneAngle": num("聚光灯锥角"),
		}, []string{"kind"})},
		{Name: "maya_render_settings", Description: "设置当前渲染器、分辨率、帧范围、Arnold 采样和图像前缀。renderer 常用 arnold、mayaSoftware、mayaHardware2。", Parameters: obj(map[string]any{
			"renderer": str("渲染器"), "width": num("宽"), "height": num("高"),
			"start": num("起始帧"), "end": num("结束帧"), "samples": num("Arnold AA 采样"), "prefix": str("图像前缀"),
		}, nil)},
		{Name: "maya_render", Description: "渲染当前帧或指定帧。Arnold 走 MtoA，其它渲染器走 maya.cmds.render。", Parameters: obj(map[string]any{
			"camera": str("摄影机变换名，默认 persp"), "frame": num("渲染前跳到的帧"),
			"width": num("宽"), "height": num("高"),
		}, nil)},
		{Name: "maya_execute", Description: "在 Maya 内执行 Python 或 MEL。Python 表达式返回值会带回；语句可写入 RESULT。", Parameters: obj(map[string]any{
			"language": enum("python 或 mel", "python", "mel"),
			"code":     str("要执行的代码"),
		}, []string{"code"})},
	}
}

func Find(name string) (Tool, bool) {
	for _, tool := range All() {
		if tool.Name == name {
			return tool, true
		}
	}
	return Tool{}, false
}

func obj(props map[string]any, required []string) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	if required == nil {
		required = []string{}
	}
	return map[string]any{
		"type":                 "object",
		"properties":           props,
		"required":             required,
		"additionalProperties": true,
	}
}

func str(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func num(desc string) map[string]any {
	return map[string]any{"type": "number", "description": desc}
}

func boolean(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

func vec(desc string) map[string]any {
	return map[string]any{"type": "array", "description": desc, "items": map[string]any{"type": "number"}}
}

func namesProp() map[string]any {
	return map[string]any{"description": "对象名列表，也可以是逗号分隔的字符串", "oneOf": []any{
		map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		map[string]any{"type": "string"},
	}}
}

func enum(desc string, values ...string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "enum": values}
}

func Timeout(name string) time.Duration {
	switch name {
	case "maya_render", "maya_simulate", "maya_execute":
		return 15 * time.Minute
	default:
		return 45 * time.Second
	}
}

func OpenAI() []map[string]any {
	out := make([]map[string]any, 0, len(All()))
	for _, tool := range All() {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.Parameters,
			},
		})
	}
	return out
}
