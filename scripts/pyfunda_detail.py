#!/usr/bin/env python3
"""Optional enrichment adapter.

Input: one Funda listing URL on argv[1]
Output: one JSON object on stdout

This intentionally delegates network behavior to the third-party `pyfunda`
package so the Go CLI does not duplicate its undocumented transport logic.
"""

import json
import sys
from datetime import datetime, timezone

from funda import Funda


def pick(obj, *names, default=None):
    for name in names:
        if obj is None:
            break
        if isinstance(obj, dict) and name in obj:
            return obj[name]
        if hasattr(obj, name):
            return getattr(obj, name)
    return default


def numeric(v):
    if v is None:
        return 0
    try:
        return float(v)
    except (TypeError, ValueError):
        return 0


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: pyfunda_detail.py <funda-url>", file=sys.stderr)
        return 2

    url = sys.argv[1]
    with Funda() as client:
        listing = client.listing(url)

    raw = pick(listing, "raw", {})
    details = pick(listing, "property_details", {})
    price_obj = pick(listing, "price", None)
    address_obj = pick(listing, "address", None)

    listing_id = str(
        pick(listing, "global_id", "globalId", "id", default="")
        or pick(raw, "globalId", "id", default="")
    )
    city = (
        pick(listing, "city", default="")
        or pick(address_obj, "city", default="")
        or pick(raw, "city", default="")
    )
    address = (
        pick(listing, "title", default="")
        or pick(address_obj, "full_address", "street_address", default="")
        or pick(raw, "title", default="")
    )
    price = numeric(pick(price_obj, "amount", default=0))
    living_area = numeric(pick(listing, "living_area", "floor_area", default=0))
    bedrooms = int(numeric(pick(listing, "bedrooms", default=0)))
    rooms = int(numeric(pick(listing, "rooms_count", default=0)))
    energy = str(pick(listing, "energy_label", default="") or "")

    out = {
        "id": listing_id,
        "url": str(pick(listing, "url", default=url) or url),
        "transaction_type": "rent" if "/huur/" in url else "buy" if "/koop/" in url else "",
        "house_type": pick(details, "house_type"),
        "city": str(city or ""),
        "address": str(address or ""),
        "price": price,
        "living_area": living_area,
        "bedrooms": bedrooms,
        "rooms": rooms,
        "energy_label": energy,
        "latitude": numeric(pick(getattr(listing, "location", None), "latitude", default=0)),
        "longitude": numeric(pick(getattr(listing, "location", None), "longitude", default=0)),
        "features": pick(details, "features"),
        "status": str(pick(listing, "status", default="") or ""),
        "fetched_at": datetime.now(timezone.utc).isoformat(),
    }
    print(json.dumps(out, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
