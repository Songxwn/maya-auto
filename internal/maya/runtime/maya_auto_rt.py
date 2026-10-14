# Injected into a live Maya 2024+ session by Maya Auto. Python 3.10+.
import json
import traceback

import maya.cmds as cmds

VERSION = "1"


def _int(value, default=0):
    if value is None or value == "":
        return default
    return int(float(value))


def _float(value, default=0.0):
    if value is None or value == "":
        return default
    return float(value)


def _vec(value, n=3, default=None):
    if default is None:
        default = [0.0] * n
    if not isinstance(value, (list, tuple)) or len(value) < n:
        return list(default)
    return [float(value[i]) for i in range(n)]


def _names(args, key="names"):
    value = args.get(key)
    if value is None or value == "":
        return []
    if isinstance(value, str):
        raw = value.replace(";", ",").split(",")
        return [item.strip() for item in raw if item.strip()]
    if isinstance(value, (list, tuple)):
        return [str(item) for item in value if str(item).strip()]
    return [str(value)]


def _first(value):
    if isinstance(value, (list, tuple)):
        return value[0] if value else value
    return value


def _path(value):
    return str(value).replace("\\", "/")


def _set_if(node, attr, value):
    if cmds.attributeQuery(attr, node=node, exists=True):
        cmds.setAttr(node + "." + attr, value)
        return True
    return False


def _cameras():
    found = []
    for shape in cmds.ls(type="camera") or []:
        parent = cmds.listRelatives(shape, parent=True) or [shape]
        if parent[0] not in found:
            found.append(parent[0])
    return found


def _transform_of(node):
    if cmds.nodeType(node) == "transform":
        return node
    parent = cmds.listRelatives(node, parent=True, fullPath=False)
    if parent:
        return parent[0]
    return node


def maya_status(_args):
    renderer = ""
    if cmds.objExists("defaultRenderGlobals"):
        renderer = cmds.getAttr("defaultRenderGlobals.currentRenderer") or ""
    return {
        "version": cmds.about(version=True),
        "apiVersion": cmds.about(apiVersion=True),
        "product": cmds.about(product=True),
        "file": cmds.file(query=True, sceneName=True) or "",
        "modified": bool(cmds.file(query=True, modified=True)),
        "time": cmds.currentTime(query=True),
        "min": cmds.playbackOptions(query=True, minTime=True),
        "max": cmds.playbackOptions(query=True, maxTime=True),
        "renderer": renderer,
        "units": cmds.currentUnit(query=True, linear=True),
        "selection": cmds.ls(selection=True) or [],
        "cameras": _cameras(),
        "counts": {
            "mesh": len(cmds.ls(type="mesh") or []),
            "light": len(cmds.ls(type="light") or []),
            "nparticle": len(cmds.ls(type="nParticle") or []),
            "camera": len(cmds.ls(type="camera") or []),
        },
        "runtime": VERSION,
    }


def maya_scene(args):
    action = args.get("action") or "query"
    if action == "new":
        cmds.file(new=True, force=True)
    elif action == "open":
        path = args.get("path")
        if not path:
            raise RuntimeError("缺少 path")
        cmds.file(_path(path), open=True, force=True)
    elif action == "save":
        cmds.file(save=True)
    elif action == "save_as":
        path = args.get("path")
        if not path:
            raise RuntimeError("缺少 path")
        file_type = args.get("type") or "mayaAscii"
        cmds.file(rename=_path(path))
        cmds.file(save=True, type=file_type)
    elif action != "query":
        raise RuntimeError("不支持的场景操作: %s" % action)
    return {"file": cmds.file(query=True, sceneName=True) or "", "action": action}


def maya_list(args):
    limit = _int(args.get("limit"), 300)
    if limit < 1:
        limit = 1
    if limit > 2000:
        limit = 2000
    if args.get("selected"):
        nodes = cmds.ls(selection=True) or []
        return {"nodes": nodes[:limit], "count": len(nodes)}
    node_type = args.get("type") or "transform"
    nodes = cmds.ls(type=node_type) or []
    return {"type": node_type, "nodes": nodes[:limit], "count": len(nodes)}


def maya_select(args):
    if args.get("clear"):
        cmds.select(clear=True)
        return {"selection": []}
    names = _names(args)
    if not names:
        raise RuntimeError("缺少 names")
    cmds.select(names, replace=not bool(args.get("add")))
    return {"selection": cmds.ls(selection=True) or []}


def maya_primitive(args):
    kind = (args.get("kind") or "cube").lower()
    makers = {
        "cube": cmds.polyCube,
        "sphere": cmds.polySphere,
        "cylinder": cmds.polyCylinder,
        "plane": cmds.polyPlane,
        "torus": cmds.polyTorus,
        "cone": cmds.polyCone,
        "pipe": cmds.polyPipe,
        "prism": cmds.polyPrism,
    }
    if kind not in makers:
        raise RuntimeError("不支持的基本体: %s" % kind)
    kwargs = {}
    if args.get("name"):
        kwargs["name"] = str(args["name"])
    created = makers[kind](**kwargs)
    transform = created[0]
    history = created[1] if len(created) > 1 else transform
    candidates = {
        "width": ["width"],
        "height": ["height"],
        "depth": ["depth"],
        "radius": ["radius"],
        "sectionRadius": ["sectionRadius"],
        "thickness": ["thickness"],
        "length": ["length"],
        "sides": ["numberOfSides"],
        "subdivisionsX": ["subdivisionsX", "subdivisionsAxis"],
        "subdivisionsY": ["subdivisionsY", "subdivisionsHeight"],
        "subdivisionsZ": ["subdivisionsZ", "subdivisionsCaps"],
    }
    integer_attrs = {"subdivisionsX", "subdivisionsY", "subdivisionsZ", "sides"}
    for key, attrs in candidates.items():
        if key not in args or args.get(key) in (None, ""):
            continue
        for attr in attrs:
            if cmds.attributeQuery(attr, node=history, exists=True):
                if key in integer_attrs:
                    cmds.setAttr(history + "." + attr, _int(args[key]))
                else:
                    cmds.setAttr(history + "." + attr, _float(args[key]))
                break
    if args.get("translate"):
        cmds.xform(transform, worldSpace=True, translation=_vec(args["translate"]))
    if args.get("rotate"):
        cmds.xform(transform, worldSpace=True, rotation=_vec(args["rotate"]))
    if args.get("scale"):
        cmds.xform(transform, worldSpace=True, scale=_vec(args["scale"], 3, [1, 1, 1]))
    return {"transform": transform, "history": history, "kind": kind}


def maya_transform(args):
    names = _names(args) or (cmds.ls(selection=True) or [])
    if not names:
        raise RuntimeError("没有可变换的对象")
    relative = bool(args.get("relative"))
    for name in names:
        if args.get("translate"):
            cmds.xform(name, worldSpace=not relative, relative=relative, translation=_vec(args["translate"]))
        if args.get("rotate"):
            cmds.xform(name, worldSpace=not relative, relative=relative, rotation=_vec(args["rotate"]))
        if args.get("scale"):
            cmds.xform(name, relative=relative, worldSpace=not relative, scale=_vec(args["scale"], 3, [1, 1, 1]))
    return {"names": names, "relative": relative}


def maya_edit(args):
    action = args.get("action") or ""
    names = _names(args) or (cmds.ls(selection=True) or [])
    if action == "delete":
        if not names:
            raise RuntimeError("没有要删除的对象")
        cmds.delete(names)
        return {"deleted": names}
    if action == "select":
        return maya_select(args)
    if not names and action not in ("delete",):
        raise RuntimeError("没有目标对象")
    if action == "extrude":
        faces = cmds.polyListComponentConversion(names, toFace=True)
        cmds.select(faces, replace=True)
        result = cmds.polyExtrudeFacet(localTranslateZ=_float(args.get("distance"), 1))
        return {"result": result}
    if action == "bevel":
        cmds.select(names, replace=True)
        result = cmds.polyBevel3(
            fraction=_float(args.get("fraction"), 0.2),
            offsetAsFraction=True,
            segments=_int(args.get("segments"), 1),
        )
        return {"result": result}
    if action == "smooth":
        cmds.select(names, replace=True)
        result = cmds.polySmooth(divisions=_int(args.get("divisions"), 1))
        return {"result": result}
    if action == "duplicate":
        result = cmds.duplicate(names, returnRootsOnly=True)
        return {"result": result}
    if action == "combine":
        result = cmds.polyUnite(names, constructionHistory=True)
        return {"result": result}
    if action == "separate":
        result = []
        for name in names:
            separated = cmds.polySeparate(name) or []
            result.extend(separated)
        return {"result": result}
    if action == "freeze":
        cmds.makeIdentity(names, apply=True, translate=True, rotate=True, scale=True, normal=False)
        return {"names": names}
    if action == "center_pivot":
        cmds.xform(names, centerPivots=True)
        return {"names": names}
    if action == "history":
        cmds.delete(names, constructionHistory=True)
        return {"names": names}
    if action == "rename":
        new_name = args.get("name")
        if not new_name:
            raise RuntimeError("缺少 name")
        renamed = cmds.rename(names[0], str(new_name))
        return {"name": renamed}
    if action == "parent":
        parent = args.get("parent")
        if parent in ("", None, "world", "|"):
            cmds.parent(names, world=True)
            return {"names": names, "parent": "world"}
        cmds.parent(names, str(parent))
        return {"names": names, "parent": parent}
    raise RuntimeError("不支持的编辑操作: %s" % action)


def _set_color(shader, rgb):
    for attr in ("baseColor", "color"):
        if cmds.attributeQuery(attr, node=shader, exists=True):
            cmds.setAttr(shader + "." + attr, rgb[0], rgb[1], rgb[2], type="double3")
            return attr
    return ""


def maya_material(args):
    name = str(args.get("name") or "mayaAutoMat")
    shader_type = args.get("type") or "standardSurface"
    rgb = _vec(args.get("color"), 3, [0.8, 0.8, 0.8])
    if cmds.objExists(name):
        shader = name
    else:
        try:
            shader = cmds.shadingNode(shader_type, asShader=True, name=name)
        except Exception:
            shader = cmds.shadingNode("lambert", asShader=True, name=name)
            shader_type = "lambert"
    sg = shader + "SG"
    if not cmds.objExists(sg):
        sg = cmds.sets(renderable=True, noSurfaceShader=True, empty=True, name=sg)
    if not cmds.isConnected(shader + ".outColor", sg + ".surfaceShader"):
        cmds.connectAttr(shader + ".outColor", sg + ".surfaceShader", force=True)
    color_attr = _set_color(shader, rgb)
    if "roughness" in args and cmds.attributeQuery("specularRoughness", node=shader, exists=True):
        cmds.setAttr(shader + ".specularRoughness", _float(args["roughness"], 0.4))
    targets = _names(args, "assignTo")
    if targets:
        cmds.sets(targets, edit=True, forceElement=sg)
    return {"shader": shader, "shadingGroup": sg, "type": shader_type, "colorAttr": color_attr}


def maya_camera(args):
    name = str(args.get("name") or "mayaAutoCam")
    created = cmds.camera(name=name)
    transform = created[0]
    shape = created[1]
    if args.get("translate"):
        cmds.xform(transform, worldSpace=True, translation=_vec(args["translate"]))
    if args.get("rotate"):
        cmds.xform(transform, worldSpace=True, rotation=_vec(args["rotate"]))
    if args.get("focalLength") is not None:
        cmds.setAttr(shape + ".focalLength", _float(args["focalLength"], 35))
    return {"transform": transform, "shape": shape}


def maya_fx(args):
    kind = (args.get("kind") or "").lower()
    if kind == "nparticle":
        position = _vec(args.get("position"), 3, [0, 0, 0])
        emitter_name = str(args.get("name") or "mayaAutoEmitter")
        particle_name = emitter_name + "Particle"
        emitter = _first(cmds.emitter(position=position, type="omni", rate=_float(args.get("rate"), 100), name=emitter_name))
        particle = _first(cmds.nParticle(name=particle_name))
        cmds.connectDynamic(particle, em=emitter)
        return {"emitter": emitter, "particle": particle}
    if kind == "ncloth":
        mesh = args.get("mesh")
        if not mesh:
            selected = cmds.ls(selection=True) or []
            if not selected:
                raise RuntimeError("缺少 mesh")
            mesh = selected[0]
        cmds.select(str(mesh), replace=True)
        cloth = cmds.nClothCreate()
        return {"mesh": mesh, "ncloth": cloth}
    if kind == "field":
        field = (args.get("field") or "gravity").lower()
        makers = {
            "gravity": cmds.gravity,
            "air": cmds.air,
            "turbulence": cmds.turbulence,
            "vortex": cmds.vortex,
            "radial": cmds.radial,
            "uniform": cmds.uniform,
            "drag": cmds.drag,
        }
        if field not in makers:
            raise RuntimeError("不支持的力场: %s" % field)
        position = _vec(args.get("position"), 3, [0, 5, 0])
        kwargs = {"position": position}
        if args.get("name"):
            kwargs["name"] = str(args["name"])
        node = _first(makers[field](**kwargs))
        magnitude = args.get("magnitude")
        if magnitude is not None:
            if not _set_if(node, "magnitude", _float(magnitude)):
                _set_if(node, "speed", _float(magnitude))
        affect = _names(args, "affect")
        for item in affect:
            cmds.connectDynamic(item, fields=node)
        return {"field": node, "kind": field, "affect": affect}
    if kind == "nucleus":
        name = str(args.get("name") or "nucleus1")
        if not cmds.objExists(name):
            raise RuntimeError("找不到 %s" % name)
        if args.get("gravity") is not None:
            cmds.setAttr(name + ".gravity", _float(args["gravity"], 9.8))
        if args.get("gravityDirection"):
            direction = _vec(args["gravityDirection"], 3, [0, -1, 0])
            cmds.setAttr(name + ".gravityDirection", direction[0], direction[1], direction[2], type="double3")
        if args.get("airDensity") is not None:
            cmds.setAttr(name + ".airDensity", _float(args["airDensity"]))
        return {"nucleus": name}
    raise RuntimeError("不支持的特效类型: %s" % kind)


def _frame_range(args):
    start = args.get("start", args.get("min"))
    end = args.get("end", args.get("max"))
    if start is None:
        start = cmds.playbackOptions(query=True, minTime=True)
    if end is None:
        end = cmds.playbackOptions(query=True, maxTime=True)
    return _float(start), _float(end)


def maya_simulate(args):
    action = args.get("action") or ""
    if action == "range":
        start, end = _frame_range(args)
        cmds.playbackOptions(minTime=start, maxTime=end, animationStartTime=start, animationEndTime=end)
        return {"min": start, "max": end}
    if action == "time":
        if args.get("time") is None:
            raise RuntimeError("缺少 time")
        cmds.currentTime(_float(args["time"]))
        return {"time": cmds.currentTime(query=True)}
    if action == "evaluate":
        start, end = _frame_range(args)
        step = _float(args.get("step"), 1) or 1
        frame = start
        guard = 0
        while frame <= end + 1e-6:
            cmds.currentTime(frame)
            frame += step
            guard += 1
            if guard > 20000:
                raise RuntimeError("解算帧数超过 20000")
        return {"start": start, "end": end, "step": step, "time": cmds.currentTime(query=True)}
    if action == "bake":
        names = _names(args) or (cmds.ls(selection=True) or [])
        if not names:
            raise RuntimeError("没有要烘焙的对象")
        start, end = _frame_range(args)
        result = cmds.bakeResults(
            names,
            time=(start, end),
            simulation=True,
            sampleBy=_float(args.get("step"), 1) or 1,
            disableImplicitControl=True,
            preserveOutsideKeys=True,
        )
        return {"baked": result, "start": start, "end": end}
    if action == "playblast":
        start, end = _frame_range(args)
        width = _int(args.get("width"), 1280)
        height = _int(args.get("height"), 720)
        import os

        path = _path(args.get("path") or (cmds.workspace(query=True, rootDirectory=True) + "movies/maya_auto"))
        parent = os.path.dirname(path)
        if parent:
            os.makedirs(parent, exist_ok=True)
        fmt = "image"
        compression = "jpg"
        lower = path.lower()
        if lower.endswith(".avi"):
            fmt = "avi"
            compression = "none"
            path = path[:-4]
        result = cmds.playblast(
            startTime=start,
            endTime=end,
            format=fmt,
            compression=compression,
            filename=path,
            forceOverwrite=True,
            width=width,
            height=height,
            percent=100,
            viewer=False,
            showOrnaments=False,
            offScreen=True,
            clearCache=True,
        )
        return {"path": result or path, "format": fmt, "start": start, "end": end}
    if action == "geometry_cache":
        names = _names(args) or (cmds.ls(selection=True) or [])
        if not names:
            raise RuntimeError("没有要缓存的对象")
        import os

        start, end = _frame_range(args)
        directory = _path(args.get("path") or (cmds.workspace(query=True, rootDirectory=True) + "cache"))
        file_name = "maya_auto"
        if directory.endswith("/"):
            directory = directory[:-1]
        else:
            file_name = directory.split("/")[-1] or "maya_auto"
            directory = "/".join(directory.split("/")[:-1]) or "."
        os.makedirs(directory, exist_ok=True)
        result = cmds.cacheFile(
            *names,
            directory=directory,
            fileName=file_name,
            startTime=start,
            endTime=end,
            format="OneFile",
            worldSpace=True,
            singleCache=True,
        )
        return {"cache": result, "directory": directory, "fileName": file_name}
    raise RuntimeError("不支持的解算操作: %s" % action)


def maya_light(args):
    kind = (args.get("kind") or "point").lower()
    name = str(args.get("name") or "")
    arnold = {"aiskydome": "aiSkyDomeLight", "aiarea": "aiAreaLight", "aiskydomelight": "aiSkyDomeLight"}
    if kind in arnold:
        try:
            if not cmds.pluginInfo("mtoa", query=True, loaded=True):
                cmds.loadPlugin("mtoa", quiet=True)
        except Exception as exc:
            raise RuntimeError("无法加载 Arnold(mtoa): %s" % exc)
        kwargs = {"asLight": True}
        if name:
            kwargs["name"] = name
        node = cmds.shadingNode(arnold[kind], **kwargs)
    else:
        makers = {
            "ambient": cmds.ambientLight,
            "directional": cmds.directionalLight,
            "point": cmds.pointLight,
            "spot": cmds.spotLight,
            "area": cmds.areaLight,
        }
        if kind not in makers:
            raise RuntimeError("不支持的灯光: %s" % kind)
        kwargs = {}
        if name:
            kwargs["name"] = name
        node = _first(makers[kind](**kwargs))
    transform = _transform_of(node)
    shape = node if node != transform else (cmds.listRelatives(transform, shapes=True) or [node])[0]
    if args.get("intensity") is not None:
        if not _set_if(shape, "intensity", _float(args["intensity"])):
            _set_if(transform, "intensity", _float(args["intensity"]))
    if args.get("color"):
        rgb = _vec(args["color"])
        target = shape if cmds.attributeQuery("color", node=shape, exists=True) else transform
        if cmds.attributeQuery("color", node=target, exists=True):
            cmds.setAttr(target + ".color", rgb[0], rgb[1], rgb[2], type="double3")
    if args.get("translate"):
        cmds.xform(transform, worldSpace=True, translation=_vec(args["translate"]))
    if args.get("rotate"):
        cmds.xform(transform, worldSpace=True, rotation=_vec(args["rotate"]))
    if kind == "spot" and args.get("coneAngle") is not None and cmds.attributeQuery("coneAngle", node=shape, exists=True):
        cmds.setAttr(shape + ".coneAngle", _float(args["coneAngle"], 40))
    return {"light": shape, "transform": transform, "kind": kind}


def maya_render_settings(args):
    renderer = args.get("renderer")
    if renderer:
        renderer = str(renderer)
        if renderer == "arnold":
            loaded = False
            try:
                loaded = bool(cmds.pluginInfo("mtoa", query=True, loaded=True))
            except Exception:
                loaded = False
            if not loaded:
                cmds.loadPlugin("mtoa", quiet=True)
        cmds.setAttr("defaultRenderGlobals.currentRenderer", renderer, type="string")
    if args.get("width"):
        cmds.setAttr("defaultResolution.width", _int(args["width"], 960))
    if args.get("height"):
        cmds.setAttr("defaultResolution.height", _int(args["height"], 540))
    if args.get("start") is not None:
        cmds.setAttr("defaultRenderGlobals.startFrame", _float(args["start"]))
    if args.get("end") is not None:
        cmds.setAttr("defaultRenderGlobals.endFrame", _float(args["end"]))
    if args.get("samples") is not None and cmds.objExists("defaultArnoldRenderOptions"):
        if cmds.attributeQuery("AASamples", node="defaultArnoldRenderOptions", exists=True):
            cmds.setAttr("defaultArnoldRenderOptions.AASamples", _int(args["samples"], 3))
    if args.get("prefix"):
        cmds.setAttr("defaultRenderGlobals.imageFilePrefix", str(args["prefix"]), type="string")
    current = cmds.getAttr("defaultRenderGlobals.currentRenderer")
    return {
        "renderer": current,
        "width": cmds.getAttr("defaultResolution.width"),
        "height": cmds.getAttr("defaultResolution.height"),
        "prefix": cmds.getAttr("defaultRenderGlobals.imageFilePrefix"),
    }


def maya_render(args):
    if args.get("frame") is not None:
        cmds.currentTime(_float(args["frame"]))
    camera = str(args.get("camera") or "persp")
    width = _int(args.get("width"), int(cmds.getAttr("defaultResolution.width")))
    height = _int(args.get("height"), int(cmds.getAttr("defaultResolution.height")))
    renderer = cmds.getAttr("defaultRenderGlobals.currentRenderer") or ""
    if renderer == "arnold":
        try:
            if not cmds.pluginInfo("mtoa", query=True, loaded=True):
                cmds.loadPlugin("mtoa", quiet=True)
        except Exception as exc:
            raise RuntimeError("无法加载 Arnold(mtoa): %s" % exc)
        try:
            import mtoa.cmds.arnoldRender as arnold_render

            arnold_render.arnoldRender(width, height, False, True, camera, "")
            return {"renderer": "arnold", "method": "mtoa.cmds.arnoldRender", "camera": camera, "width": width, "height": height}
        except Exception as first:
            try:
                image = cmds.arnoldRender(camera=camera, width=width, height=height)
                return {"renderer": "arnold", "method": "cmds.arnoldRender", "camera": camera, "image": image}
            except Exception as second:
                raise RuntimeError("Arnold 渲染失败: %s | %s" % (first, second))
    image = cmds.render(camera, x=width, y=height)
    return {"renderer": renderer, "camera": camera, "image": image, "width": width, "height": height}


def maya_execute(args):
    language = (args.get("language") or "python").lower()
    code = args.get("code") or ""
    if not str(code).strip():
        raise RuntimeError("代码为空")
    if language == "mel":
        import maya.mel as mel

        return {"language": "mel", "result": mel.eval(str(code))}
    import io
    import sys

    stdout = io.StringIO()
    previous = sys.stdout
    sys.stdout = stdout
    namespace = {"cmds": cmds, "RESULT": None}
    try:
        try:
            result = eval(compile(str(code), "<maya-auto>", "eval"), namespace, namespace)
            mode = "eval"
        except SyntaxError:
            exec(compile(str(code), "<maya-auto>", "exec"), namespace, namespace)
            result = namespace.get("RESULT")
            mode = "exec"
        text = stdout.getvalue()
        if len(text) > 8000:
            text = text[:8000] + "\n...truncated"
        return {"language": "python", "mode": mode, "result": result, "stdout": text}
    finally:
        sys.stdout = previous


DISPATCH = {
    "maya_status": maya_status,
    "maya_scene": maya_scene,
    "maya_list": maya_list,
    "maya_select": maya_select,
    "maya_primitive": maya_primitive,
    "maya_transform": maya_transform,
    "maya_edit": maya_edit,
    "maya_material": maya_material,
    "maya_camera": maya_camera,
    "maya_fx": maya_fx,
    "maya_simulate": maya_simulate,
    "maya_light": maya_light,
    "maya_render_settings": maya_render_settings,
    "maya_render": maya_render,
    "maya_execute": maya_execute,
}


def dispatch_b64(payload):
    import base64

    try:
        data = json.loads(base64.b64decode(payload).decode("utf-8"))
        name = data.get("name") or ""
        args = data.get("args") or {}
        if not isinstance(args, dict):
            raise RuntimeError("args 必须是对象")
        fn = DISPATCH.get(name)
        if fn is None:
            raise RuntimeError("未知工具: %s" % name)
        cmds.undoInfo(openChunk=True, chunkName="maya-auto " + name)
        try:
            result = fn(args)
        finally:
            cmds.undoInfo(closeChunk=True)
        if name not in ("maya_status", "maya_list", "maya_select"):
            try:
                cmds.refresh(force=True)
            except Exception:
                pass
        return json.dumps({"ok": True, "result": result}, ensure_ascii=False, default=str)
    except Exception:
        return json.dumps({"ok": False, "error": traceback.format_exc()}, ensure_ascii=False)
