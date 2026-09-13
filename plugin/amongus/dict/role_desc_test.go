package dict

import "testing"

func TestGetRoleDescExtraRoles(t *testing.T) {
	tests := map[string]string{
		"灵光师": "简介：你看到其他人都是小黑人，使用技能探查他们的阵营\n\n详细介绍：灵光师可以感知其他玩家的存在，在游戏的最开始，所有人物在你的眼里都是白色的，而使用技能可以让你得知他的阵营。绿色代表船员，灰色代表中立，红色代表内鬼。然而作为代价，你无法得知这个人究竟是谁。\n\n开场白：人与人的悲欢并不相通，我只觉得他们吵闹。",
		"天使":  "简介：可以看到双方词语的好人\n\n详细介绍：天使隶属于好人阵营，可以同时知道好坏双方的词语。但是天使不知道哪一个是好人词哪一个是坏人词，你需要带领好人阵营推理出坏人和白板",
		"白板":  "简介：不知道词语，猜测获得胜利\n\n详细介绍：白板隶属于中立阵营，开局不知道任何一方的词语。你需要在白天伪造发言并且推测出好人和坏人的词语。在夜晚通过猜测双方词语获得胜利，猜错则继续游戏，本晚不能继续猜测",
	}

	for role, want := range tests {
		t.Run(role, func(t *testing.T) {
			if got := GetRoleDesc(role); got != want {
				t.Fatalf("GetRoleDesc(%q) = %q, want %q", role, got, want)
			}
		})
	}
}

func TestUndercoverRolesAreNotAmongUsRoles(t *testing.T) {
	for _, role := range []string{"天使", "白板"} {
		if _, ok := RoleTextReverse[role]; ok {
			t.Fatalf("谁是卧底职业 %q 不应出现在 Among Us 职业映射中", role)
		}
	}
}
