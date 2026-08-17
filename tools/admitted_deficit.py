import json,glob,sys,collections
def role_group(role):
    head,_,tail=role.rpartition("_")
    return head if head and tail.isdigit() else role
def load(d):
    g=json.load(open(f'{d}/greeks.json'))
    roles={(r["venue_id"],r["client_id"]):role_group(r["role"]) for r in g["terminal_accounts"]}
    adm=collections.Counter()
    for r in g["request_budgets"]: adm[role_group(r["role"])]+=r["admitted"]
    fills=collections.Counter(); qty=collections.Counter()
    for f in glob.glob(f'{d}/venues/*/**/*.jsonl',recursive=True):
        for line in open(f):
            if '"OrderFill"' not in line: continue
            e=json.loads(line)
            if e.get("event")!="OrderFill": continue
            v=(e.get("data") or {}).get("venue_id"); p=(e.get("data") or {}).get("payload") or {}
            r=roles.get((v,e.get("client_id")))
            if r is None: continue
            fills[r]+=1; qty[r]+=p.get("filled_qty") or 0
    return adm,fills,qty
roles={'tri':'triangle_arb','carry':'dated_carry_arb','meta':'metaorder_trader'}
print(f'{"class":18s}{"seed":>5}{"admCtl":>9}{"admBud":>9}{"deficit":>9}{"fillCtl":>9}{"fillBud":>9}{"fillLoss":>10}{"ratio":>7}')
for tag,role in roles.items():
    for s in (91,92):
        ac,fc,_=load(f'logs/def_control_{s}'); ab,fb,_=load(f'logs/def_{tag}_{s}')
        A,B=ac[role],ab[role]; F,G=fc[role],fb[role]
        d=1-B/A if A else 0; fl=1-G/F if F else 0
        print(f'{role:18s}{s:5d}{A:9d}{B:9d}{100*d:8.1f}%{F:9d}{G:9d}{100*fl:9.1f}%{(fl/d if d else float("nan")):7.2f}')
