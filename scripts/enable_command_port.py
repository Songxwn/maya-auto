# Run once inside Maya 2024+ Script Editor (Python).
# Maya Auto talks to this localhost command port. Do not expose it beyond this machine.
import maya.cmds as cmds

port = "127.0.0.1:7001"
try:
    if not cmds.commandPort(port, query=True):
        cmds.commandPort(name=port, sourceType="mel", echoOutput=False)
    print("maya-auto commandPort %s" % port)
except Exception as exc:
    print("maya-auto commandPort failed: %s" % exc)
    raise
