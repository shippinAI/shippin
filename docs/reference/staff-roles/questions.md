# Questions only the operator can answer

1. **Who is staff today?** Names, emails, and which roles each should get. Is "Hades" a person,
   a shared account or a bot? Shared accounts can't hold staff roles (AC-2 needs individual accounts).
2. **Which roles exist now?** Is the 7-role catalog right, or fewer (e.g. merge Platform Admin into
   Owner, Security into Owner) for a small team?
3. **Owners:** who besides the operator? Two-person rules and the last-Owner guard need >= 2.
4. **Two-person rule:** required for Owner/Security grants, tenant delete, break-glass? Or off until the team grows?
5. **Break-glass self-activation fallback** when no approver answers: allowed (with red review) or never?
6. Can Platform Admin request break-glass, or only Ops/Owner?
7. **IAM_OWNER holders:** who keeps Nomen instance ownership, and is a sealed recovery account wanted?
8. SoD waivers: OK for one person to be Owner + Security while the team is tiny?
9. Mail, DNS, Billing panel sections: which exist or are planned (not in `operator.go` today)?
10. Machine users / CI: do any need a staff role?
11. External auditor access: expected, and default expiry (30/90 days)?
12. Idle threshold for flagging staff accounts (90 days proposed) and access review cadence.
