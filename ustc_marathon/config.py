import re
from pathlib import Path
from typing import Literal

import tomlkit
from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

from .errors import Problem

ROOT = Path(__file__).resolve().parent.parent
RUNTIME = ROOT / "runtime"
CONFIG_PATH = ROOT / "config.toml"


class StrictModel(BaseModel):
    model_config = ConfigDict(extra="forbid", str_strip_whitespace=True, hide_input_in_errors=True)


class Monitor(StrictModel):
    username: str = ""
    poll_interval_seconds: float = Field(default=30.0, gt=0, allow_inf_nan=False)
    request_timeout_seconds: float = Field(default=30.0, gt=0, allow_inf_nan=False)
    login_timeout_seconds: float = Field(default=600.0, gt=0, allow_inf_nan=False)
    headless: bool = True


SIZES = ("XS(160/84A)", "S(165/88A)", "M(170/92A)", "L(175/96A)",
         "XL(180/100A)", "2XL(185/104A)", "3XL(190/108A)")


class Profile(StrictModel):
    depart_name: str = ""
    user_type: Literal["学生", "教职工"] = "学生"
    sex: Literal["", "男", "女"] = ""
    size: str = ""
    id_number: str = ""
    phone: str = ""
    emergency_contact: str = ""

    def require_complete(self) -> None:
        missing = [name for name in type(self).model_fields if not getattr(self, name)]
        if missing:
            raise Problem("请填写 profile 配置字段：" + "、".join(missing))
        if self.size not in SIZES:
            raise Problem("size 必须填写网站使用的完整衣服尺码，参见 README.md。")
        for name in ("phone", "emergency_contact"):
            if re.fullmatch(r"1[3-9][0-9]{9}", getattr(self, name)) is None:
                raise Problem(f"{name} 必须填写有效的十一位手机号码。")
        if re.fullmatch(r"[1-9][0-9]{5}(18|19|20)[0-9]{2}(0[1-9]|1[0-2])(0[1-9]|[12][0-9]|3[01])[0-9]{3}[0-9Xx]", self.id_number) is None:
            raise Problem("id_number 不符合网站要求的十八位身份证号格式。")


class Target(StrictModel):
    id: str = Field(min_length=1)
    name: str = Field(min_length=1)
    campus: Literal["西校区", "高新校区"]

    @field_validator("id")
    @classmethod
    def validate_id(cls, value: str) -> str:
        if re.fullmatch(r"[A-Za-z0-9_-]+", value) is None:
            raise ValueError("比赛 id 含有不允许的字符。")
        return value


class Config(StrictModel):
    monitor: Monitor = Field(default_factory=Monitor)
    profile: Profile = Field(default_factory=Profile)
    targets: list[Target] = Field(default_factory=list)

    @model_validator(mode="after")
    def unique_targets(self) -> "Config":
        ids = [target.id for target in self.targets]
        if len(ids) != len(set(ids)):
            raise ValueError("目标比赛 id 重复。")
        return self

    def require_watch(self) -> None:
        if not self.targets:
            raise Problem("请先运行 select 命令选择目标比赛。")
        if not self.monitor.username:
            raise Problem("请填写 monitor.username，或者运行 credentials set。")
        self.profile.require_complete()


def read_config(path: Path = CONFIG_PATH) -> Config:
    if not path.is_file():
        raise Problem("缺少 config.toml，请运行 setup.ps1 创建配置文件。")
    return Config.model_validate(tomlkit.parse(path.read_text(encoding="utf-8")).unwrap())


def write_setting(section: str, key: str, value: object, path: Path = CONFIG_PATH) -> None:
    document = tomlkit.parse(path.read_text(encoding="utf-8"))
    if section:
        document[section][key] = value
    else:
        document[key] = value
    Config.model_validate(document.unwrap())
    # 写入同一目录的中间文件，完成后一次替换配置。
    pending = path.with_suffix(".toml.pending")
    pending.write_text(tomlkit.dumps(document), encoding="utf-8")
    pending.replace(path)
