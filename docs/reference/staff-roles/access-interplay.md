# Roles vs consent access and break-glass

The 50bcc84 model stays: no implicit tenant data access for anyone; consent grants approved by
the customer; break-glass with fresh login (10 min), 1 h duration, red audit, customer notice
(legal hold may delay up to 30 days). This design only decides **who** may do each step.

| Step | Allowed roles | Notes |
|---|---|---|
| Request consent grant | Owner, Platform Admin, Ops, Support | Customer approves; unchanged |
| End own grant | requester | Customer may revoke any time |
| Request break-glass | Owner, Ops **[guess: Platform Admin too?]** | Fresh login, justification, category |
| Approve break-glass | Security, Owner | **Never the requester** (no self-approval) |
| Legal hold (withhold notice) | Security only | Max 30 days, audited `breakglass.notice_withheld` |
| Read all audit incl. break-glass | Security, Auditor, Owner | Others see own entries |
| Superowner (IAM_OWNER) path | IAM_OWNER holders | Separate, see `nomen-mapping.md` |

## Break-glass flow change
Today break-glass is effective immediately for the platform admin. Proposed:
1. Requester activates with reauth + justification → state `pending_approval`.
2. Approver (Security/Owner, != requester) approves with their own fresh login → `active` for 1 h.
3. **Emergency fallback** when no approver responds in 15 min **[guess]**: requester may self-activate,
   marked `breakglass.unapproved` (red), and Security must review after the fact within 24 h.
   the operator decides if this fallback exists (see `questions.md`).
- Support never gets break-glass: customer-help cases go through consent.
- Approval and legal-hold decisions are separate permissions so one person need not hold both.
