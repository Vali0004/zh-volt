-- Define o protocolo
local olt_proto = Proto("olt", "Custom OLT Protocol")

-- Define os campos do cabeçalho com base na struct Packet (sources/packet.go)
local f_magic   = ProtoField.bytes("olt.magic", "Magic", base.SPACE)
local f_request = ProtoField.uint16("olt.request", "Request ID", base.DEC)
local f_type    = ProtoField.uint16("olt.type", "Type", base.HEX)
local f_flag3   = ProtoField.uint8("olt.flag3", "Flag 3 (Old Status)", base.HEX)
local f_flag0   = ProtoField.uint8("olt.flag0", "Flag 0", base.HEX)
local f_flag1   = ProtoField.uint8("olt.flag1", "Flag 1", base.HEX)
local f_flag2   = ProtoField.uint8("olt.flag2", "Flag 2", base.HEX)

-- Campos gerados (calculados)
local f_calc_id = ProtoField.uint16("olt.calc_id", "Calculated ID", base.DEC)
local f_data    = ProtoField.bytes("olt.data", "Data Payload", base.SPACE)

-- Registra os campos no protocolo
olt_proto.fields = { f_magic, f_request, f_type, f_flag3, f_flag0, f_flag1, f_flag2, f_calc_id, f_data }

-- Função principal do Dissector
function olt_proto.dissector(buffer, pinfo, tree)
    local length = buffer:len()

    -- O cabeçalho completo tem 12 bytes:
    -- 4 (Magic) + 2 (RequestID) + 2 (RequestType) + 1 (Flag3) + 1 (Flag0) + 1 (Flag1) + 1 (Flag2)
    if length < 12 then return end

    -- Verifica os bytes "OltMagic" (b9 58 d6 3a) conforme definido no Go
    local magic_tvb = buffer(0, 4)
    if magic_tvb:uint() ~= 0xb958d63a then
        return
    end

    -- Configuração da coluna de protocolo
    pinfo.cols.protocol = "OLT"

    -- Criação da árvore de detalhes
    local subtree = tree:add(olt_proto, buffer(), "OLT Protocol Data")

    -- Adiciona os campos seguindo a ordem de UnmarshalBinary
    subtree:add(f_magic, buffer(0, 4))
    subtree:add(f_request, buffer(4, 2))

    local type_val = buffer(6, 2):uint()
    subtree:add(f_type, buffer(6, 2))

    subtree:add(f_flag3, buffer(8, 1))
    local f0_val = buffer(9, 1):uint()
    subtree:add(f_flag0, buffer(9, 1))
    local f1_val = buffer(10, 1):uint()
    subtree:add(f_flag1, buffer(10, 1))
    local f2_val = buffer(11, 1):uint()
    subtree:add(f_flag2, buffer(11, 1))

    -- Implementação da lógica da função Id() do Go:
    -- id = RequestType >> (Flag0 >> 8) + (Flag1 | Flag2)
    -- Nota: Como Flag0 é uint8, Flag0 >> 8 é sempre 0.
    local shift_amt = bit.rshift(f0_val, 8)
    local calc_id = bit.rshift(type_val, shift_amt) + bit.bor(f1_val, f2_val)

    local id_item = subtree:add(f_calc_id, buffer(6, 6), calc_id)
    id_item:set_generated()

    -- Payload (Data)
    if length > 12 then
        subtree:add(f_data, buffer(12, length - 12))
    end

    -- Atualiza a coluna de informações para facilitar a visualização
    pinfo.cols.info = string.format("Req: %d, Type: 0x%X, ID: %d", buffer(4, 2):uint(), type_val, calc_id)
end

-- Associa ao EtherType configurado (0x88b6)
local eth_table = DissectorTable.get("ethertype")
eth_table:add(0x88b6, olt_proto)