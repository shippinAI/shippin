# Sources

Cited from public docs; URLs not re-fetched during this design pass **[verify before build]**.

- Upstream docs (Nomen is built on Zitadel), Roles and authorizations: https://zitadel.com/docs/guides/manage/console/roles
- Upstream docs, Granted projects / project grants: https://zitadel.com/docs/guides/manage/console/projects
- Upstream docs, Retrieve user roles (claims `urn:nomen:iam:org:project:{id}:roles`):
  https://zitadel.com/docs/guides/integrate/retrieve-user-roles
- Upstream docs, Claims reference: https://zitadel.com/docs/apis/openidoauth/claims
- Upstream docs, Login policy / MFA: https://zitadel.com/docs/guides/manage/console/default-settings
- DigitalOcean, Team roles (Owner, Member, Biller, Modifier, Resource viewer, Billing viewer):
  https://docs.digitalocean.com/platform/teams/roles/
- AWS IAM security best practices (least privilege, MFA, no root use):
  https://docs.aws.amazon.com/IAM/latest/UserGuide/best-practices.html
- AWS root user guidance (root as break-glass): https://docs.aws.amazon.com/IAM/latest/UserGuide/root-user-best-practices.html
- GitHub, Roles in an organization (owners, members, security managers, billing managers):
  https://docs.github.com/en/organizations/managing-peoples-access-to-your-organization-with-roles/roles-in-an-organization
- GitHub, recommendation to keep >= 2 owners: same page.
- NIST SP 800-53 Rev. 5: AC-2 Account Management, AC-5 Separation of Duties, AC-6 Least Privilege
  (incl. AC-6(1), AC-6(5), AC-6(9) log privileged functions):
  https://csrc.nist.gov/pubs/sp/800/53/r5/upd1/final
- OWASP Authorization Cheat Sheet (deny by default, server-side checks):
  https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html
