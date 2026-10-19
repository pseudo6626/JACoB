#!/usr/bin/env python3
from __future__ import annotations
import argparse, hashlib, json, re, sys
from pathlib import Path

MAX_HTML_BYTES=4*1024*1024
PACKAGE_RE=re.compile(r'^community\.[a-z0-9][a-z0-9-]{0,47}\.[a-f0-9]{6}$')
SEMVER_RE=re.compile(r'^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$')
ALLOWED_CAPS={'clipboard','control','network','recorder','overlay','video','vision','intertab'}
ALLOWED_CATEGORIES={'accessibility','bgs','combat','engineering','exploration','fleet-carrier','hud','mining','navigation','racing','trading','utility','other'}


def load(path): return json.loads(path.read_text(encoding='utf-8'))

def fail(msg): raise ValueError(msg)

def validate_manifest(root,path):
    m=load(path)
    if m.get('schema')!=1: fail(f'{path}: schema must be 1')
    pid=str(m.get('id',''))
    if not PACKAGE_RE.fullmatch(pid): fail(f'{path}: invalid id')
    if path.name != pid+'.json': fail(f'{path}: filename must match id')
    if not str(m.get('name','')).strip(): fail(f'{path}: missing name')
    if not str(m.get('description','')).strip(): fail(f'{path}: missing description')
    if not str(m.get('author','')).strip(): fail(f'{path}: missing author')
    maintainers=m.get('maintainers')
    if not isinstance(maintainers,list) or not maintainers or any(not str(x).strip() for x in maintainers): fail(f'{path}: maintainers required')
    if not SEMVER_RE.fullmatch(str(m.get('version',''))): fail(f'{path}: invalid version')
    sdk=str(m.get('jacobSdk',''))
    if not re.fullmatch(r'>=\d+',sdk): fail(f'{path}: jacobSdk must look like >=15')
    category=str(m.get('category','utility')).strip().lower()
    if category not in ALLOWED_CATEGORIES: fail(f'{path}: invalid category')
    caps=m.get('capabilities',[])
    if not isinstance(caps,list) or any(c not in ALLOWED_CAPS for c in caps): fail(f'{path}: unknown capability')
    if caps != sorted(set(caps)): fail(f'{path}: capabilities must be sorted and unique')
    art=m.get('artifact')
    if not isinstance(art,dict): fail(f'{path}: artifact required')
    rel=str(art.get('path',''))
    expected=f"registry/packages/{pid}/{m['version']}.html"
    if rel!=expected: fail(f'{path}: artifact.path must be {expected}')
    ap=(root/rel).resolve(); rr=root.resolve()
    if rr not in ap.parents: fail(f'{path}: artifact escapes repository')
    if not ap.exists(): fail(f'{path}: artifact is missing')
    raw=ap.read_bytes()
    if not raw or len(raw)>MAX_HTML_BYTES: fail(f'{path}: artifact size invalid')
    if len(raw)!=art.get('sizeBytes'): fail(f'{path}: artifact sizeBytes mismatch')
    sha=hashlib.sha256(raw).hexdigest()
    if sha!=str(art.get('sha256','')).lower(): fail(f'{path}: artifact sha256 mismatch')
    derived=m.get('derivedFrom')
    if derived is not None:
        if not isinstance(derived,dict) or not PACKAGE_RE.fullmatch(str(derived.get('id',''))) or not SEMVER_RE.fullmatch(str(derived.get('version',''))):
            fail(f'{path}: invalid derivedFrom')
    return m


def main():
    p=argparse.ArgumentParser();p.add_argument('--root',type=Path,default=Path('.'));a=p.parse_args();root=a.root
    tabs_dir=root/'registry'/'tabs'; manifests=[]
    try:
        for path in sorted(tabs_dir.glob('*.json')): manifests.append(validate_manifest(root,path))
        idx_path=root/'registry'/'index.json';idx=load(idx_path)
        if idx.get('schema')!=1: fail('registry/index.json: schema must be 1')
        if idx.get('tabs')!=manifests: fail('registry/index.json is stale; rebuild it from registry/tabs')
        ids={m['id'] for m in manifests}
        stats_path=root/'registry'/'stats.json'
        if stats_path.exists():
            stats=load(stats_path)
            if stats.get('schema')!=1 or not isinstance(stats.get('downloads',{}),dict): fail('registry/stats.json: invalid format')
            for key,value in stats.get('downloads',{}).items():
                if key not in ids: fail(f'registry/stats.json: unknown package {key}')
                if not isinstance(value,int) or value<0: fail(f'registry/stats.json: invalid download count for {key}')
        for m in manifests:
            d=m.get('derivedFrom')
            if d and d['id'] not in ids: fail(f"{m['id']}: derivedFrom package {d['id']} is missing")
    except (ValueError,OSError,json.JSONDecodeError) as e:
        print('ERROR:',e,file=sys.stderr);return 2
    print(f'Community registry valid: {len(manifests)} tab(s).')
    return 0
if __name__=='__main__': raise SystemExit(main())
