from pathlib import Path
import struct, sys

def u16(b,o): return struct.unpack_from('<H',b,o)[0]
def u32(b,o): return struct.unpack_from('<I',b,o)[0]
def p16(b,o,v): struct.pack_into('<H',b,o,v)
def p32(b,o,v): struct.pack_into('<I',b,o,v)
def align(v,a): return (v+a-1)//a*a

def read_ico(path):
    b=Path(path).read_bytes()
    r,t,n=struct.unpack_from('<HHH',b,0)
    if r or t!=1 or not n: raise ValueError('invalid ico')
    icons=[]
    for i in range(n):
        e=struct.unpack_from('<BBBBHHII',b,6+i*16)
        w,h,cc,res,planes,bpp,size,off=e
        icons.append((w,h,cc,res,planes,bpp,b[off:off+size]))
    return icons

def resource_blob(icons, base_rva):
    n=len(icons)
    off=0
    root=off; off+=16+2*8
    icon_type=off; off+=16+n*8
    group_type=off; off+=16+8
    icon_lang=[]
    for _ in icons: icon_lang.append(off); off+=24
    group_lang=off; off+=24
    data_entries=[]
    for _ in range(n+1): data_entries.append(off); off+=16
    off=align(off,4)
    group_off=off
    group=bytearray(struct.pack('<HHH',0,1,n))
    for i,(w,h,cc,res,planes,bpp,data) in enumerate(icons,1):
        group += struct.pack('<BBBBHHIH',w,h,cc,res,planes,bpp,len(data),i)
    off+=len(group); off=align(off,4)
    icon_data=[]
    for x in icons:
        icon_data.append(off); off+=len(x[6]); off=align(off,4)
    blob=bytearray(off)
    def directory(at,count): struct.pack_into('<IIHHHH',blob,at,0,0,0,0,0,count)
    def entry(at,idv,target,sub): struct.pack_into('<II',blob,at,idv,target | (0x80000000 if sub else 0))
    directory(root,2)
    entry(root+16,3,icon_type,True); entry(root+24,14,group_type,True)
    directory(icon_type,n)
    for i,lang in enumerate(icon_lang,1): entry(icon_type+16+(i-1)*8,i,lang,True)
    directory(group_type,1); entry(group_type+16,1,group_lang,True)
    for i,lang in enumerate(icon_lang):
        directory(lang,1); entry(lang+16,1033,data_entries[i],False)
    directory(group_lang,1); entry(group_lang+16,1033,data_entries[-1],False)
    for i,x in enumerate(icons):
        struct.pack_into('<IIII',blob,data_entries[i],base_rva+icon_data[i],len(x[6]),0,0)
        blob[icon_data[i]:icon_data[i]+len(x[6])]=x[6]
    struct.pack_into('<IIII',blob,data_entries[-1],base_rva+group_off,len(group),0,0)
    blob[group_off:group_off+len(group)]=group
    return bytes(blob)

def patch(exe, ico):
    path=Path(exe); b=bytearray(path.read_bytes())
    pe=u32(b,0x3c)
    if b[pe:pe+4]!=b'PE\0\0': raise ValueError('not PE')
    fh=pe+4; nsec=u16(b,fh+2); optsize=u16(b,fh+16); opt=fh+20
    if u16(b,opt)!=0x20b: raise ValueError('PE32+ required')
    sect_align=u32(b,opt+32); file_align=u32(b,opt+36); headers=u32(b,opt+60)
    st=opt+optsize
    new_hdr=st+nsec*40
    if new_hdr+40>headers: raise ValueError('no section-header room')
    max_va=0
    for i in range(nsec):
        sh=st+i*40; vs=u32(b,sh+8); va=u32(b,sh+12); rs=u32(b,sh+16)
        max_va=max(max_va,va+max(vs,rs))
    new_va=align(max_va,sect_align)
    blob=resource_blob(read_ico(ico),new_va)
    raw=align(len(b),file_align); raw_size=align(len(blob),file_align)
    if len(b)<raw: b.extend(b'\0'*(raw-len(b)))
    b.extend(blob); b.extend(b'\0'*(raw_size-len(blob)))
    name=b'.rsrc\0\0\0'
    sh=struct.pack('<8sIIIIIIHHI',name,len(blob),new_va,raw_size,raw,0,0,0,0,0x40000040)
    b[new_hdr:new_hdr+40]=sh
    p16(b,fh+2,nsec+1)
    p32(b,opt+56,align(new_va+len(blob),sect_align))
    dd=opt+112+2*8
    p32(b,dd,new_va); p32(b,dd+4,len(blob))
    p32(b,opt+64,0)
    path.write_bytes(b)

if __name__=='__main__':
    patch(sys.argv[1],sys.argv[2])
