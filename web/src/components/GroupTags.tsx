import { Space, Tag, Tooltip } from "antd";

export type GroupSummary = { id: string; name: string };

export function GroupTags({ groups }: { groups: GroupSummary[] }) {
  if (groups.length === 0) return <span>—</span>;
  return (
    <Space size={[0, 4]} wrap>
      {groups.slice(0, 3).map((group) => (
        <Tag key={group.id}>{group.name}</Tag>
      ))}
      {groups.length > 3 && (
        <Tooltip title={groups.map((group) => group.name).join(", ")}>
          <Tag>+{groups.length - 3}</Tag>
        </Tooltip>
      )}
    </Space>
  );
}
