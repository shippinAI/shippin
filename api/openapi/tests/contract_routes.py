"""Resolve local OpenAPI references and export path/method evidence; no network access."""
import json, pathlib, sys
import yaml
root=pathlib.Path(sys.argv[1]).resolve().parent
cache={}
def document(path):
    path=path.resolve()
    if not path.is_relative_to(root): raise ValueError("reference outside contract owner")
    if path not in cache: cache[path]=yaml.safe_load(path.read_text(encoding="utf-8"))
    return cache[path]
def resolve(ref,base):
    file,_,pointer=ref.partition("#")
    if ":" in file: raise ValueError("remote references are not allowed in this check")
    path=(base.parent/file).resolve() if file else base
    node=document(path)
    for part in pointer.strip("/").split("/") if pointer.strip("/") else []:
        node=node[part.replace("~1","/").replace("~0","~")]
    return node,path
def item(node,base):
    if "$ref" in node:
        target,base=resolve(node["$ref"],base)
        node={**item(target,base),**{k:v for k,v in node.items() if k!="$ref"}}
    return node
def check(node,base):
    if isinstance(node,dict):
        if "$ref" in node: resolve(node["$ref"],base)
        for v in node.values(): check(v,base)
    elif isinstance(node,list):
        for v in node: check(v,base)
spec=pathlib.Path(sys.argv[1]).resolve(); data=document(spec); out={}
for path,raw in data["paths"].items():
    entry=item(raw,spec); ops={}
    for method in ("get","post","put","patch","delete","head","options","trace"):
        if method in entry:
            op=entry[method]
            ops[method.upper()]={"status":op.get("x-implementation-status",entry.get("x-implementation-status","registered")),"reason":op.get("x-implementation-note",entry.get("x-implementation-note",""))}
    out[path]=ops
for path in root.rglob("*.yaml"): check(document(path),path)
print(json.dumps(out))
