package notify

import "text/template"

var payoutAuthorizedSubject = template.Must(template.New("subject").Parse(
	`Your claim {{.PolicyNumber}}: payment of {{.Amount}} is on its way`))

var payoutAuthorizedBody = template.Must(template.New("body").Parse(`Hello,

We have approved your claim under policy {{.PolicyNumber}} and authorized a payment of
{{.Amount}} to the account ending {{.IBANLast4}}. It should reach your account by {{.ValueDate}}.

If the account details are no longer right, reply to this message before {{.ValueDate}}.

Tidewater Mutual claims
`))
