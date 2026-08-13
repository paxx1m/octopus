import { useState } from "react"
import { motion } from "motion/react"
import { useTranslations } from "use-intl"
import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { useChangePassword, useAuth } from "@/api/endpoints/user"
import Logo from "@/components/modules/logo"
import { toast } from "@/components/common/Toast"
import { validatePasswordChange } from "@/lib/validators"

export function ForceChangePassword() {
  const t = useTranslations("login")
  const tSetting = useTranslations("setting")
  const { logout } = useAuth()
  const changePassword = useChangePassword()

  const [oldPassword, setOldPassword] = useState("")
  const [newPassword, setNewPassword] = useState("")
  const [confirmPassword, setConfirmPassword] = useState("")
  const [error, setError] = useState<string | null>(null)

    const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)

    const errKey = validatePasswordChange(oldPassword, newPassword, confirmPassword);
    if (errKey) {
      setError(tSetting(`account.password.${errKey}`));
      return;
    }
    if (newPassword === "admin") {
      setError(t("forceChange.defaultForbidden"))
      return
    }

    try {
      await changePassword.mutateAsync({ oldPassword, newPassword })
      toast.success(tSetting("account.password.success"))
      logout()
    } catch (err: unknown) {
      const msg =
        err && typeof err === "object" && "message" in err && typeof (err as { message: unknown }).message === "string"
          ? (err as { message: string }).message
          : tSetting("account.password.failed")
      setError(msg)
    }
  }

  return (
    <motion.div
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
      transition={{ duration: 0.3 }}
      className="min-h-screen flex items-center justify-center px-6 text-foreground"
    >
      <div className="w-full max-w-sm space-y-8">
        <header className="flex flex-col items-center gap-3">
          <Logo size={48} />
          <h1 className="text-2xl font-bold">{t("forceChange.title")}</h1>
          <p className="text-sm text-muted-foreground text-center">{t("forceChange.hint")}</p>
        </header>

        <form onSubmit={handleSubmit} className="space-y-4">
          <Field>
            <FieldLabel htmlFor="old-password">{tSetting("account.password.oldPlaceholder")}</FieldLabel>
            <Input
              id="old-password"
              type="password"
              value={oldPassword}
              onChange={(e) => setOldPassword(e.target.value)}
              required
              disabled={changePassword.isPending}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="new-password">{tSetting("account.password.newPlaceholder")}</FieldLabel>
            <Input
              id="new-password"
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              required
              disabled={changePassword.isPending}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="confirm-password">{tSetting("account.password.confirmPlaceholder")}</FieldLabel>
            <Input
              id="confirm-password"
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              required
              disabled={changePassword.isPending}
            />
          </Field>

          {error && <FieldDescription className="text-destructive">{error}</FieldDescription>}

          <Button type="submit" disabled={changePassword.isPending} className="w-full">
            {changePassword.isPending ? tSetting("account.saving") : t("forceChange.submit")}
          </Button>
        </form>
      </div>
    </motion.div>
  )
}
