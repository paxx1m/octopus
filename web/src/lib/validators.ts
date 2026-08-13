/**
 * 密码修改校验：返回错误 i18n key 或 null（校验通过）。
 *
 * 统一 ForceChangePassword 与 Account 中的重复校验逻辑。
 */
export function validatePasswordChange(
    oldPassword: string,
    newPassword: string,
    confirmPassword: string,
): string | null {
    if (!oldPassword.trim()) return 'oldEmpty';
    if (!newPassword.trim()) return 'newEmpty';
    if (newPassword !== confirmPassword) return 'mismatch';
    if (newPassword.length < 6) return 'tooShort';
    return null;
}
