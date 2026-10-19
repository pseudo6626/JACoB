#!/usr/bin/env python3
from __future__ import annotations
import argparse, json, sys
from datetime import datetime, timezone
from pathlib import Path

PREFIX='community--'
SEPARATOR='--v'

def main():
    p=argparse.ArgumentParser();p.add_argument('--root',type=Path,default=Path('.'));a=p.parse_args()
    downloads={}
    for raw in sys.stdin:
        raw=raw.rstrip('\n')
        if not raw: continue
        try: tag,count=raw.split('\t',1); count=int(count)
        except ValueError: continue
        if not tag.startswith(PREFIX) or SEPARATOR not in tag: continue
        package=tag[len(PREFIX):].rsplit(SEPARATOR,1)[0]
        if not package.startswith('community.'): continue
        downloads[package]=downloads.get(package,0)+max(0,count)
    path=a.root/'registry'/'stats.json'; path.parent.mkdir(parents=True,exist_ok=True)
    old={}
    if path.exists():
        try: old=json.loads(path.read_text(encoding='utf-8'))
        except Exception: old={}
    if old.get('schema')==1 and old.get('downloads')==downloads:
        print('Community download statistics unchanged.')
        return 0
    data={'schema':1,'generatedAt':datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace('+00:00','Z'),'downloads':dict(sorted(downloads.items()))}
    path.write_text(json.dumps(data,indent=2,ensure_ascii=False)+'\n',encoding='utf-8')
    print(f'Updated download statistics for {len(downloads)} package(s).')
    return 0
if __name__=='__main__': raise SystemExit(main())
