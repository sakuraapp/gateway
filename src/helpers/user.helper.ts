import { UserInfo } from '@sakuraapp/common'
import { User } from '@sakuraapp/shared'

export class UserHelper {
    static build(user: User): UserInfo {
        return {
            id: user.id,
            ...user.profile,
        }
    }
}
