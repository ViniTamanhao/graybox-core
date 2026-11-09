"""Independent black-box contracts. Never copy this file into a coding trial."""
import json
import urllib.request
import urllib.error


def request(url, method, path, payload=None, credential=None):
    data = None if payload is None else (payload.encode() if isinstance(payload, str) else json.dumps(payload).encode())
    headers = {"Content-Type": "application/json"}
    if credential:
        headers["Authorization"] = credential
    req = urllib.request.Request(url + path, data=data, headers=headers, method=method)
    try:
        response = urllib.request.urlopen(req, timeout=5)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        return response.status, json.load(response)


def grade(url, task, auth, other):
    results = []
    def check(name, method, path, payload, expected, credential=auth, critical=False):
        try:
            actual = request(url, method, path, payload, credential)
            ok = actual == expected
            results.append(dict(name=name, passed=ok, critical=critical, actual=actual, expected=expected))
            return actual
        except Exception as error:
            results.append(dict(name=name, passed=False, critical=critical, error=type(error).__name__))
    check("authentication", "GET", "/products/1", None, (401, {"error":"unauthorized"}), None, True)
    check("invalid-auth", "GET", "/orders/40", None, (401, {"error":"unauthorized"}), "Bearer invalid", True)
    check("product", "GET", "/products/1", None, (200, {"id":1,"price_cents":1999}))
    check("ownership", "GET", "/orders/41", None, (403, {"error":"forbidden"}), critical=True)
    check("cancel-ownership", "POST", "/orders/41/cancel", None, (403, {"error":"forbidden"}), critical=True)
    for product,qty,code,sub,disc in [(1,1,"SAVE10",1999,200),(2,1,"SAVE10",5,1),(2,3,"SAVE10",15,2),(3,100,"SAVE10",1000000,100000),(1,2,"",3998,0)]:
        check(f"price-{product}-{qty}-{code}","POST","/discounts/apply",dict(product_id=product,quantity=qty,code=code),(200,dict(subtotal_cents=sub,discount_cents=disc,total_cents=sub-disc)))
    for payload,error in [(dict(product_id=1,quantity=0),"invalid_quantity"),(dict(product_id=1,quantity=101),"invalid_quantity"),(dict(product_id=99,quantity=1),"invalid_product"),(dict(product_id=1,quantity=1,code="NO"),"invalid_discount"),('{"quantity":1,"unknown":true}',"invalid_json"),('{} {}',"invalid_json")]:
        check("validation-"+str(payload),"POST","/orders",payload,(400,dict(error=error)))
    for product,qty in [(1,5),(3,1),(1,6),(2,100)]:
        sub={1:1999,2:5,3:10000}[product]*qty
        expected=(400,dict(error="invalid_discount"))
        if task=="feature" and sub>=10000:
            disc=(sub*15+50)//100
            expected=(200,dict(subtotal_cents=sub,discount_cents=disc,total_cents=sub-disc))
        check(f"loyal-{product}-{qty}","POST","/discounts/apply",dict(product_id=product,quantity=qty,code="LOYAL15"),expected)
    order=dict(id=43,state="pending",subtotal_cents=3998,discount_cents=400,total_cents=3598)
    check("create","POST","/orders",dict(product_id=1,quantity=2,code="SAVE10"),(201,order))
    check("persisted-create","GET","/orders/43",None,(200,order))
    order={**order,"state":"cancelled"}
    check("cancel","POST","/orders/43/cancel",None,(200,order))
    check("persisted-cancel","GET","/orders/43",None,(200,order))
    check("repeat-cancel","POST","/orders/43/cancel",None,(409,dict(error="invalid_state")),critical=True)
    check("shipped-cancel","POST","/orders/42/cancel",None,(409,dict(error="invalid_state")),critical=True)
    check("shipped-preserved","GET","/orders/42",None,(200,dict(id=42,state="shipped",subtotal_cents=10000,discount_cents=1000,total_cents=9000)),critical=True)
    check("other-owner-preserved","GET","/orders/41",None,(200,dict(id=41,state="pending",subtotal_cents=5,discount_cents=0,total_cents=5)),other,True)
    return dict(passed=sum(x['passed'] for x in results),total=len(results),critical_failures=sum(x['critical'] and not x['passed'] for x in results),results=results)
