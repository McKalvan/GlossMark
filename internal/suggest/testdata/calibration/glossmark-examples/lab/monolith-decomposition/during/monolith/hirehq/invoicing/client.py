import requests


class InvoicingClient:
    def __init__(self, base_url, timeout=5.0):
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout

    def issue(self, hire):
        resp = requests.post(
            f"{self.base_url}/v1/invoices",
            json={
                "hire_id": hire.pk,
                "customer_account": hire.customer.account_number,
                "vat_registered": hire.customer.vat_registered,
                "day_rate_pence": hire.equipment.day_rate_pence,
                "starts_on": hire.starts_on.isoformat(),
                "returned_on": hire.returned_on.isoformat(),
            },
            timeout=self.timeout,
        )
        resp.raise_for_status()
        return resp.json()
