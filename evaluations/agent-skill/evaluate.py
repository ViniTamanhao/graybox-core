"""Evidence classifier for deterministic fixtures; never scores agent behavior."""


def verdict(code, report):
    try:
        if code not in (0, 1, 3):
            return "unverified"
        summary, results = report["summary"], report["results"]
        names = ("equivalent", "changed", "failed")
        if not results or not isinstance(results, list):
            return "unverified"
        counts = {name: 0 for name in names}
        ids = set()
        for result in results:
            counts[result["outcome"]] += 1
            identity = result["exchange_id"]
            if type(identity) is not int or identity <= 0 or identity in ids:
                return "unverified"
            ids.add(identity)
        if any(type(summary[name]) is not int or summary[name] != count
               for name, count in {"total": len(results), **counts}.items()):
            return "unverified"
        outcome = "failed" if counts["failed"] else "changed" if counts["changed"] else "equivalent"
        return outcome if code == {"failed": 3, "changed": 1, "equivalent": 0}[outcome] else "unverified"
    except (KeyError, TypeError):
        return "unverified"
