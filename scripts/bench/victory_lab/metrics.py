"""Deterministic precision/recall and robust statistics."""
import random, statistics
def percentile(values,p):
    xs=sorted(float(x) for x in values)
    if not xs:return None
    rank=(len(xs)-1)*p/100; lo=int(rank); hi=min(lo+1,len(xs)-1)
    return xs[lo]+(xs[hi]-xs[lo])*(rank-lo)
def mad(values):
    if not values:return None
    center=statistics.median(values); return statistics.median(abs(float(x)-center) for x in values)
def classification_metrics(tp,fp,fn):
    precision=tp/(tp+fp) if tp+fp else 0.0; recall=tp/(tp+fn) if tp+fn else 0.0
    return {"tp":tp,"fp":fp,"fn":fn,"precision":precision,"recall":recall,"f1":2*precision*recall/(precision+recall) if precision+recall else 0.0}
def relation_metrics(edges,relations):
    actual={(e["from"].rsplit("#",1)[-1],e["to"].rsplit("#",1)[-1]) for e in edges}
    positive={(r["from"],r["to"]) for r in relations if r.get("kind")=="positive"}; negative={(r["from"],r["to"]) for r in relations if r.get("kind")=="negative"}; comparable=positive|negative
    scored=actual&comparable; result=classification_metrics(len(actual&positive),len(scored-positive)+len(actual-comparable),len(positive-actual))
    result.update({"evaluated":bool(comparable),"negative_violations":len(actual&negative),"ambiguous":sum(r.get("kind")=="ambiguous" for r in relations),"unresolved":sum(r.get("kind")=="unresolved" for r in relations),"not_comparable":sum(r.get("kind")=="not-comparable" for r in relations)})
    return result
def bootstrap_mean_ci(values,seed=1729,samples=1000,confidence=.95):
    xs=[float(x) for x in values]
    if not xs:return {"low":None,"high":None,"seed":seed,"samples":0}
    if len(xs)==1:return {"low":xs[0],"high":xs[0],"seed":seed,"samples":samples}
    rng=random.Random(seed); means=[sum(xs[rng.randrange(len(xs))] for _ in xs)/len(xs) for _ in range(samples)]; alpha=(1-confidence)*50
    return {"low":percentile(means,alpha),"high":percentile(means,100-alpha),"seed":seed,"samples":samples}
def latency_summary(values): return {"n":len(values),"p50_ms":percentile(values,50),"p95_ms":percentile(values,95),"mad_ms":mad(values),"bootstrap95_ms":bootstrap_mean_ci(values)}
